// Package frontend implements the Bubble Tea terminal user interface:
// connect screen, file browser with preview pane, settings form and the
// import progress display. All camera I/O runs in tea.Cmd goroutines so the
// UI never blocks.
package frontend

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
	"github.com/subhashraveendran/aero-shutter/internal/config"
	"github.com/subhashraveendran/aero-shutter/internal/database"
	"github.com/subhashraveendran/aero-shutter/internal/importer"
	"github.com/subhashraveendran/aero-shutter/internal/thumbnail"
)

// screen enumerates the top-level UI screens.
type screen int

const (
	screenConnect screen = iota
	screenBrowser
	screenSettings
)

// listFilter cycles the display filter with the f key.
var listFilters = []importer.Filter{
	importer.FilterAll, importer.FilterNew, importer.FilterRAW, importer.FilterJPEG,
}

// Model is the root Bubble Tea model.
type Model struct {
	cfg    config.Config
	db     *database.DB
	cam    *camera.Camera
	thumbs *thumbnail.Fetcher
	proto  thumbnail.Protocol

	width, height int
	screen        screen

	// Connect screen.
	spin       spinner.Model
	ipInput    textinput.Model
	detecting  bool
	connecting bool
	connectErr string

	// Browser state.
	files      []camera.File
	imported   map[string]bool
	storages   []camera.Storage
	battery    int
	today      int
	cursor     int
	offset     int
	selected   map[uint32]bool
	filterIdx  int
	refreshing bool
	watch      bool

	// Thumbnail for the file under the cursor.
	thumbHandle uint32
	thumbData   []byte

	// Import state.
	importing    bool
	importCh     <-chan importer.Event
	cancelImport context.CancelFunc
	lastEv       importer.Event
	prog         progress.Model

	// Overlays.
	previewOverlay bool
	detailOverlay  bool
	detailView     viewport.Model

	// Settings form.
	setInputs [2]textinput.Model // save folder, camera IP
	setAuto   bool
	setOpen   bool
	setFocus  int

	// Toasts.
	toast    string
	toastErr bool
	toastID  int

	quitting bool
}

// New builds the root model.
func New(cfg config.Config, db *database.DB) Model {
	cam := camera.New()

	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(styleAccent))

	ip := textinput.New()
	ip.Placeholder = cfg.CameraIP
	ip.Prompt = "camera ip > "
	ip.CharLimit = 45
	ip.Width = 30

	pr := progress.New(progress.WithDefaultGradient())
	pr.ShowPercentage = false

	var setInputs [2]textinput.Model
	for i := range setInputs {
		setInputs[i] = textinput.New()
		setInputs[i].CharLimit = 200
		setInputs[i].Width = 44
	}

	return Model{
		cfg:       cfg,
		db:        db,
		cam:       cam,
		thumbs:    thumbnail.NewFetcher(cam, thumbnail.DefaultCacheSize),
		proto:     thumbnail.DetectProtocol(),
		screen:    screenConnect,
		spin:      sp,
		ipInput:   ip,
		prog:      pr,
		battery:   -1,
		selected:  map[uint32]bool{},
		imported:  map[string]bool{},
		setInputs: setInputs,
	}
}

// Init starts auto-detection on launch.
func (m Model) Init() tea.Cmd {
	m.detecting = true
	return tea.Batch(
		m.spin.Tick,
		detectCmd(m.cfg.CameraIP, m.cfg.LastConnected, camera.D5300Profile.DefaultIP),
	)
}

// Update routes messages to the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.prog.Width = max(10, m.width-40)
		m.detailView.Width = max(20, m.width-20)
		m.detailView.Height = max(5, m.height-10)
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m.quit()
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		pm, cmd := m.prog.Update(msg)
		if p, ok := pm.(progress.Model); ok {
			m.prog = p
		}
		return m, cmd

	case toastClearMsg:
		if msg.id == m.toastID {
			m.toast = ""
		}
		return m, nil

	case openedMsg:
		if msg.err != nil {
			return m.showToast("open failed: "+msg.err.Error(), true)
		}
		return m, nil
	}

	switch m.screen {
	case screenConnect:
		return m.updateConnect(msg)
	case screenSettings:
		return m.updateSettings(msg)
	default:
		return m.updateBrowser(msg)
	}
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.cancelImport != nil {
		m.cancelImport()
	}
	if m.cam.Connected() {
		go m.cam.Disconnect()
	}
	m.quitting = true
	return m, tea.Quit
}

func (m Model) showToast(text string, isErr bool) (tea.Model, tea.Cmd) {
	m.toast = text
	m.toastErr = isErr
	m.toastID++
	return m, toastClearCmd(m.toastID)
}

// ---- Connect screen ----------------------------------------------------

func (m Model) updateConnect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			if !m.ipInput.Focused() {
				return m.quit()
			}
		case "esc":
			m.ipInput.Blur()
			return m, nil
		case "enter":
			addr := strings.TrimSpace(m.ipInput.Value())
			if addr == "" {
				addr = m.cfg.CameraIP
			}
			m.connecting = true
			m.connectErr = ""
			return m, tea.Batch(m.spin.Tick, connectCmd(m.cam, addr))
		case "d":
			if !m.ipInput.Focused() {
				m.detecting = true
				m.connectErr = ""
				return m, tea.Batch(m.spin.Tick, detectCmd(m.cfg.CameraIP, m.cfg.LastConnected, camera.D5300Profile.DefaultIP))
			}
		case "tab", "m", "i":
			if !m.ipInput.Focused() {
				return m, m.ipInput.Focus()
			}
		}
		var cmd tea.Cmd
		m.ipInput, cmd = m.ipInput.Update(msg)
		return m, cmd

	case detectedMsg:
		m.detecting = false
		if msg.err != nil {
			m.connectErr = msg.err.Error()
			return m, m.ipInput.Focus()
		}
		m.connecting = true
		return m, tea.Batch(m.spin.Tick, connectCmd(m.cam, msg.addr))

	case connectedMsg:
		m.connecting = false
		if msg.err != nil {
			m.connectErr = msg.err.Error()
			return m, m.ipInput.Focus()
		}
		m.cfg.LastConnected = msg.addr
		_ = config.Save(m.cfg)
		m.screen = screenBrowser
		m.refreshing = true
		var cmds []tea.Cmd
		cmds = append(cmds, m.spin.Tick, refreshCmd(m.cam, m.db))
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// ---- Browser screen ----------------------------------------------------

func (m Model) updateBrowser(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.browserKey(msg)

	case refreshMsg:
		m.refreshing = false
		if msg.err != nil {
			if !m.cam.Connected() {
				m.screen = screenConnect
				m.connectErr = "connection lost: " + msg.err.Error()
				return m, nil
			}
			return m.showToast("refresh failed: "+msg.err.Error(), true)
		}
		hadNew := len(msg.files) > len(m.files)
		m.files = msg.files
		m.imported = msg.imported
		m.storages = msg.storages
		m.battery = msg.battery
		m.today = msg.today
		m.clampCursor()
		var cmds []tea.Cmd
		if c := m.fetchCursorThumb(); c != nil {
			cmds = append(cmds, c)
		}
		if m.watch && m.cfg.AutoImport && hadNew && !m.importing {
			cmds = append(cmds, m.startImport(importer.FilterNew))
		}
		return m, tea.Batch(cmds...)

	case thumbMsg:
		if msg.err == nil && msg.handle == m.currentHandle() {
			m.thumbHandle = msg.handle
			m.thumbData = msg.data
		}
		return m, nil

	case importEventMsg:
		return m.handleImportEvent(msg)

	case watchTickMsg:
		if !m.watch || m.screen != screenBrowser {
			return m, nil
		}
		cmds := []tea.Cmd{watchTickCmd()}
		if !m.importing && !m.refreshing {
			m.refreshing = true
			cmds = append(cmds, refreshCmd(m.cam, m.db))
		}
		return m, tea.Batch(cmds...)
	}

	if m.detailOverlay {
		var cmd tea.Cmd
		m.detailView, cmd = m.detailView.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) browserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Overlays swallow most keys.
	if m.previewOverlay || m.detailOverlay {
		switch key {
		case "esc", "q", "P", "D", "enter":
			m.previewOverlay = false
			m.detailOverlay = false
			return m, nil
		}
		if m.detailOverlay {
			var cmd tea.Cmd
			m.detailView, cmd = m.detailView.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch key {
	case "q":
		return m.quit()
	case "up", "k":
		m.moveCursor(-1)
		return m, m.fetchCursorThumb()
	case "down", "j":
		m.moveCursor(1)
		return m, m.fetchCursorThumb()
	case "pgup":
		m.moveCursor(-m.listHeight())
		return m, m.fetchCursorThumb()
	case "pgdown":
		m.moveCursor(m.listHeight())
		return m, m.fetchCursorThumb()
	case "home", "g":
		m.cursor = 0
		m.clampCursor()
		return m, m.fetchCursorThumb()
	case "end", "G":
		m.cursor = len(m.visibleFiles()) - 1
		m.clampCursor()
		return m, m.fetchCursorThumb()
	case " ":
		if f, ok := m.currentFile(); ok {
			m.selected[f.Handle] = !m.selected[f.Handle]
			m.moveCursor(1)
		}
		return m, nil
	case "enter":
		return m, m.fetchCursorThumb()
	case "r":
		if !m.refreshing && !m.importing {
			m.refreshing = true
			return m, tea.Batch(m.spin.Tick, refreshCmd(m.cam, m.db))
		}
		return m, nil
	case "f":
		m.filterIdx = (m.filterIdx + 1) % len(listFilters)
		m.cursor, m.offset = 0, 0
		return m, m.fetchCursorThumb()
	case "i":
		if !m.importing {
			return m, m.startImport(importer.FilterNew)
		}
		return m, nil
	case "a":
		if !m.importing {
			return m, m.startImport(importer.FilterAll)
		}
		return m, nil
	case "S":
		if !m.importing && len(m.selectedHandles()) > 0 {
			return m, m.startImport(importer.FilterSelected)
		}
		return m.showToast("nothing selected (use space)", false)
	case "x", "esc":
		if m.importing && m.cancelImport != nil {
			m.cancelImport()
			return m.showToast("cancelling import…", false)
		}
		return m, nil
	case "s":
		if !m.importing {
			return m.openSettings()
		}
		return m, nil
	case "P":
		m.previewOverlay = true
		return m, m.fetchCursorThumb()
	case "D":
		m.detailOverlay = true
		m.detailView.SetContent(m.detailContent())
		return m, nil
	case "O":
		if f, ok := m.currentFile(); ok {
			dest, err := m.db.DestPath(f.Handle, f.Name, f.Size)
			if err == nil && dest != "" {
				return m, openFileCmd(dest)
			}
			return m.showToast("file not imported yet", false)
		}
		return m, nil
	case "w":
		m.watch = !m.watch
		if m.watch {
			mm, cmd := m.showToast("watch mode on: polling every 5s", false)
			model := mm.(Model)
			return model, tea.Batch(cmd, watchTickCmd())
		}
		return m.showToast("watch mode off", false)
	}
	return m, nil
}

func (m *Model) startImport(filter importer.Filter) tea.Cmd {
	sel := map[uint32]bool{}
	for h, on := range m.selected {
		if on {
			sel[h] = true
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelImport = cancel
	im := importer.New(m.cam, m.db)
	m.importCh = im.Run(ctx, m.files, importer.Options{
		Filter:     filter,
		Selected:   sel,
		SaveFolder: m.cfg.SaveFolder,
	})
	m.importing = true
	m.lastEv = importer.Event{}
	return tea.Batch(m.prog.SetPercent(0), waitImportEvent(m.importCh))
}

func (m Model) handleImportEvent(msg importEventMsg) (tea.Model, tea.Cmd) {
	if !msg.ok {
		// Channel closed: run finished (AllDone/Error already seen).
		m.importing = false
		if m.cancelImport != nil {
			m.cancelImport()
			m.cancelImport = nil
		}
		m.refreshing = true
		return m, refreshCmd(m.cam, m.db)
	}
	ev := msg.ev
	m.lastEv = ev
	cmds := []tea.Cmd{waitImportEvent(m.importCh)}

	switch ev.Kind {
	case importer.EventFileProgress, importer.EventFileDone, importer.EventFileSkipped:
		if ev.TotalBytes > 0 {
			cmds = append(cmds, m.prog.SetPercent(float64(ev.TotalBytesDone)/float64(ev.TotalBytes)))
		}
	case importer.EventAllDone:
		mm, cmd := m.showToast(fmt.Sprintf("imported %d file(s)", ev.Count), false)
		model := mm.(Model)
		if model.cfg.OpenAfterImport && ev.Count > 0 {
			cmds = append(cmds, openFileCmd(model.cfg.SaveFolder))
		}
		cmds = append(cmds, cmd)
		return model, tea.Batch(cmds...)
	case importer.EventError:
		if ev.Err != nil {
			mm, cmd := m.showToast("import error: "+ev.Err.Error(), true)
			model := mm.(Model)
			cmds = append(cmds, cmd)
			return model, tea.Batch(cmds...)
		}
	}
	return m, tea.Batch(cmds...)
}

// ---- Settings screen ---------------------------------------------------

func (m Model) openSettings() (tea.Model, tea.Cmd) {
	m.screen = screenSettings
	m.setInputs[0].SetValue(m.cfg.SaveFolder)
	m.setInputs[1].SetValue(m.cfg.CameraIP)
	m.setAuto = m.cfg.AutoImport
	m.setOpen = m.cfg.OpenAfterImport
	m.setFocus = 0
	return m, m.setInputs[0].Focus()
}

const settingsFieldCount = 4

func (m Model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "esc":
		m.screen = screenBrowser
		m.blurSettings()
		return m, nil
	case "tab", "down":
		return m.focusSettings(m.setFocus + 1)
	case "shift+tab", "up":
		return m.focusSettings(m.setFocus - 1)
	case "enter":
		if m.setFocus < len(m.setInputs)-1 {
			return m.focusSettings(m.setFocus + 1)
		}
		return m.saveSettings()
	case " ":
		switch m.setFocus {
		case 2:
			m.setAuto = !m.setAuto
			return m, nil
		case 3:
			m.setOpen = !m.setOpen
			return m, nil
		}
	}
	if m.setFocus < len(m.setInputs) {
		var cmd tea.Cmd
		m.setInputs[m.setFocus], cmd = m.setInputs[m.setFocus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) focusSettings(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 {
		idx = settingsFieldCount - 1
	}
	idx %= settingsFieldCount
	m.blurSettings()
	m.setFocus = idx
	if idx < len(m.setInputs) {
		return m, m.setInputs[idx].Focus()
	}
	return m, nil
}

func (m *Model) blurSettings() {
	for i := range m.setInputs {
		m.setInputs[i].Blur()
	}
}

func (m Model) saveSettings() (tea.Model, tea.Cmd) {
	if v := strings.TrimSpace(m.setInputs[0].Value()); v != "" {
		m.cfg.SaveFolder = v
	}
	if v := strings.TrimSpace(m.setInputs[1].Value()); v != "" {
		m.cfg.CameraIP = v
	}
	m.cfg.AutoImport = m.setAuto
	m.cfg.OpenAfterImport = m.setOpen
	if err := config.Save(m.cfg); err != nil {
		return m.showToast("could not save settings: "+err.Error(), true)
	}
	m.screen = screenBrowser
	m.blurSettings()
	return m.showToast("settings saved", false)
}

// ---- Helpers -----------------------------------------------------------

// visibleFiles applies the display filter to the camera inventory.
func (m Model) visibleFiles() []camera.File {
	filter := listFilters[m.filterIdx]
	if filter == importer.FilterAll {
		return m.files
	}
	out := make([]camera.File, 0, len(m.files))
	for _, f := range m.files {
		if importer.Matches(f, importer.Options{Filter: filter}, m.imported) {
			out = append(out, f)
		}
	}
	return out
}

func (m Model) currentFile() (camera.File, bool) {
	files := m.visibleFiles()
	if m.cursor < 0 || m.cursor >= len(files) {
		return camera.File{}, false
	}
	return files[m.cursor], true
}

func (m Model) currentHandle() uint32 {
	if f, ok := m.currentFile(); ok {
		return f.Handle
	}
	return 0
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	m.clampCursor()
}

func (m *Model) clampCursor() {
	n := len(m.visibleFiles())
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	h := m.listHeight()
	if h < 1 {
		h = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// fetchCursorThumb requests the thumbnail for the file under the cursor if
// it is not already loaded.
func (m *Model) fetchCursorThumb() tea.Cmd {
	f, ok := m.currentFile()
	if !ok {
		return nil
	}
	if data, hit := m.thumbs.Cached(f.Handle); hit {
		m.thumbHandle = f.Handle
		m.thumbData = data
		return nil
	}
	return thumbCmd(m.thumbs, f.Handle)
}

func (m Model) selectedHandles() []uint32 {
	var out []uint32
	for h, on := range m.selected {
		if on {
			out = append(out, h)
		}
	}
	return out
}

// humanBytes formats a byte count for display.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// humanSpeed formats bytes/second for display.
func humanSpeed(bps float64) string {
	if bps <= 0 {
		return "--"
	}
	return humanBytes(int64(bps)) + "/s"
}

// Run starts the Bubble Tea program in the alternate screen.
func Run(cfg config.Config, db *database.DB) error {
	p := tea.NewProgram(New(cfg, db), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
