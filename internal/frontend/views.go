package frontend

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
	"github.com/subhashraveendran/aero-shutter/internal/database"
	"github.com/subhashraveendran/aero-shutter/internal/importer"
	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
	"github.com/subhashraveendran/aero-shutter/internal/thumbnail"
)

// Fixed chrome heights.
const (
	topBarHeight    = 1
	bottomBarHeight = 3
	hintBarHeight   = 1 // "click anything · keys optional" discoverability line
	paneChromeRows  = 4 // border top/bottom + title line + filter-chip row
)

// toolbarHeight is the number of screen rows the browser toolbar occupies at
// the current terminal width (it wraps on narrow terminals).
func (m Model) toolbarHeight() int {
	_, lines := layoutToolbar(m.browserToolbar(), m.width, 0)
	return lines
}

// listHeight is the number of file rows visible in the left pane.
func (m Model) listHeight() int {
	h := m.height - topBarHeight - bottomBarHeight - hintBarHeight - m.toolbarHeight() - paneChromeRows
	if h < 1 {
		return 1
	}
	return h
}

// View renders the whole UI.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "loading…"
	}
	switch m.screen {
	case screenConnect:
		return m.viewConnect()
	case screenSettings:
		return m.viewSettings()
	default:
		return m.viewBrowser()
	}
}

// ---- Connect screen ----------------------------------------------------

// asciiLogo is the AERO-SHUTTER wordmark, one string per row. It is 39 cells
// wide; below narrowLogoWidth the plain title is shown instead.
var asciiLogo = []string{
	"┌─┐┌─┐┬─┐┌─┐   ┌─┐┬ ┬┬ ┬┌┬┐┌┬┐┌─┐┬─┐",
	"├─┤├┤ ├┬┘│ │───└─┐├─┤│ │ │  │ ├┤ ├┬┘",
	"┴ ┴└─┘┴└─└─┘   └─┘┴ ┴└─┘ ┴  ┴ └─┘┴└─",
}

// narrowLogoWidth is the terminal width below which the big wordmark is
// replaced by a plain one-line title.
const narrowLogoWidth = 60

// connectSteps is the "How to connect" walkthrough shown on the connect
// screen.
var connectSteps = []string{
	"Camera: MENU → Setup → Wi-Fi → Enable",
	"Computer: join Wi-Fi \"Nikon_WU2_…\"",
	"aero-shutter finds it automatically",
}

// connectNote reminds users that the camera's Wi-Fi replaces their normal
// network connection.
const connectNote = "Joining the camera's Wi-Fi disconnects normal internet — " +
	"use Ethernet or a USB Wi-Fi adapter to stay online."

func (m Model) viewConnect() string {
	if m.picker {
		return m.viewPicker()
	}
	var sections []string

	// Wordmark with a vertical gradient; plain title on narrow terminals.
	if m.width >= narrowLogoWidth {
		rows := make([]string, len(asciiLogo))
		for i, line := range asciiLogo {
			rows[i] = lipgloss.NewStyle().Foreground(logoColors[i%len(logoColors)]).Bold(true).Render(line)
		}
		sections = append(sections, strings.Join(rows, "\n"))
	} else {
		sections = append(sections, styleTitle.Render("aero-shutter"))
	}
	sections = append(sections, styleTagline.Render("Wi-Fi photo importer for Nikon"), "")

	// Status card: spinner, error, manual IP input and key hints.
	var card strings.Builder
	switch {
	case m.detecting:
		card.WriteString(m.spin.View() + " Searching for camera…")
	case m.connecting:
		card.WriteString(m.spin.View() + " Connecting…")
	default:
		card.WriteString(styleSubtle.Render("No camera connected."))
	}
	card.WriteString("\n\n")
	if m.connectErr != "" {
		card.WriteString(styleErrText.Render("✗ "+m.connectErr) + "\n\n")
	}
	card.WriteString(m.ipInput.View() + "\n\n")
	card.WriteString(styleHint.Render("click a button below · keys optional"))

	cardW := min(m.width-4, 52)
	sections = append(sections, styleConnectCard.Width(cardW).Render(card.String()))

	// The walkthrough and the note are dropped on small terminals so the
	// card always stays visible.
	if m.height >= 20 && m.width >= narrowLogoWidth {
		stepLines := []string{styleSubtle.Bold(true).Render("How to connect")}
		for i, s := range connectSteps {
			stepLines = append(stepLines,
				fmt.Sprintf("%s  %s", styleStepNum.Render(fmt.Sprintf(" %d", i+1)), styleStepText.Render(s)))
		}
		sections = append(sections, "", lipgloss.JoinVertical(lipgloss.Left, stepLines...))
		sections = append(sections, "", styleNote.Width(cardW).Align(lipgloss.Center).Render(connectNote))
	}

	content := lipgloss.JoinVertical(lipgloss.Center, sections...)
	centered := lipgloss.Place(m.width, m.height-m.connectToolbarHeight(), lipgloss.Center, lipgloss.Center, content)
	placed, lines := m.connectToolbarLayout()
	return lipgloss.JoinVertical(lipgloss.Left, centered, m.renderToolbar(placed, lines))
}

// connectToolbarHeight is the number of screen rows the connect toolbar takes.
func (m Model) connectToolbarHeight() int {
	_, lines := layoutToolbar(m.connectToolbar(), m.width, 0)
	return lines
}

// ---- Browser -----------------------------------------------------------

func (m Model) viewBrowser() string {
	top := m.viewTopBar()
	body := m.viewBody()
	bottom := m.viewBottomBar()
	hint := m.viewHint()
	toolbar := m.viewToolbar()
	view := lipgloss.JoinVertical(lipgloss.Left, top, body, bottom, hint, toolbar)

	if m.ctrlOverlay {
		return m.renderOverlay(view, m.viewControlOverlay())
	}
	if m.cheatOverlay {
		return m.renderOverlay(view, m.viewCheatOverlay())
	}
	if m.previewOverlay {
		return m.renderOverlay(view, m.viewPreviewOverlay())
	}
	if m.detailOverlay {
		return m.renderOverlay(view, m.viewDetailOverlay())
	}
	return view
}

// viewToolbar renders the clickable button toolbar at the bottom of the browser
// screen. The buttons and their hit zones come from the same browserToolbar
// slice, so a click always lands on the button under the cursor.
func (m Model) viewToolbar() string {
	placed, lines := m.browserToolbarLayout()
	return m.renderToolbar(placed, lines)
}

// viewHint is the subtle discoverability line above the toolbar.
func (m Model) viewHint() string {
	return styleHint.MaxWidth(m.width).Render("click anything · keys optional")
}

func (m Model) viewTopBar() string {
	dot := styleDisconnected.Render("● offline")
	model := "no camera"
	if m.cam.Connected() {
		dot = styleConnected.Render("● connected")
		model = m.cam.DeviceInfo().Model
		if model == "" {
			model = m.cam.Profile().Name
		}
	}
	batt := "batt --"
	if m.battery >= 0 {
		batt = fmt.Sprintf("batt %d%%", m.battery)
	}
	wifi := "wifi " + m.cam.Addr()
	if !m.cam.Connected() {
		wifi = "wifi --"
	}
	count := fmt.Sprintf("%d files", len(m.files))
	if m.watch {
		count += " · watching"
	}

	left := fmt.Sprintf("%s  %s", dot, styleTopBar.Render(model))
	right := styleTopBarKey.Render(fmt.Sprintf("%s  ·  %s  ·  %s", batt, wifi, count))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	bar := left + styleTopBar.Render(strings.Repeat(" ", gap)) + right
	return styleTopBar.MaxWidth(m.width).Render(bar)
}

func (m Model) viewBody() string {
	bodyH := m.height - topBarHeight - bottomBarHeight - hintBarHeight - m.toolbarHeight()
	if bodyH < 5 {
		bodyH = 5
	}
	listW := m.listPaneWidth()
	prevW := m.width - listW
	if prevW < 20 {
		prevW = 20
	}

	left := m.viewFileList(listW-2, bodyH-2)
	right := m.viewPreview(prevW-2, bodyH-2)

	leftPane := stylePaneFocused.Width(listW - 2).Height(bodyH - 2).Render(left)
	rightPane := stylePane.Width(prevW - 2).Height(bodyH - 2).Render(right)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

func (m Model) viewFileList(w, h int) string {
	files := m.visibleFiles()

	rows := make([]string, 0, h)
	rows = append(rows, stylePaneTitle.Render(fmt.Sprintf(" Files (%d) ", len(files))))
	rows = append(rows, m.renderFilterChips())

	// One line each is reserved for the top/bottom "more" indicators; the
	// list body gets whatever is left.
	visible := h - 2 - 2
	if visible < 1 {
		visible = 1
	}

	// Top overflow indicator.
	if m.offset > 0 {
		rows = append(rows, m.renderMoreIndicator(true))
	} else {
		rows = append(rows, "")
	}

	if m.refreshing && len(files) == 0 {
		rows = append(rows, " "+m.spin.View()+" reading card…")
	} else if len(files) == 0 {
		rows = append(rows, styleDimText.Render("  no files match this filter"))
	}

	end := min(m.offset+visible, len(files))
	for i := m.offset; i < end; i++ {
		rows = append(rows, m.renderRow(files[i], i == m.cursor, w))
	}

	// Pad so the bottom indicator sits at a stable row.
	for len(rows) < h-1 {
		rows = append(rows, "")
	}
	// Bottom overflow indicator.
	if end < len(files) {
		rows = append(rows, m.renderMoreIndicator(false))
	} else {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

// renderMoreIndicator renders the clickable "▲ more / ▼ more" overflow hint,
// highlighted while hovered.
func (m Model) renderMoreIndicator(top bool) string {
	id, text := moreDownID, "  ▼ more"
	if top {
		id, text = moreUpID, "  ▲ more"
	}
	style := styleDimText
	if m.hoverZone == id {
		style = styleAccent.Bold(true)
	}
	return style.Render(text)
}

func (m Model) renderRow(f camera.File, cursor bool, w int) string {
	check := styleDimText.Render("☐ ")
	if m.selected[f.Handle] {
		check = styleCheckOn.Render("☑ ")
	}
	badge := formatBadge(f.Format)
	mark := "  "
	if m.imported[database.Key(f.Handle, f.Name, f.Size)] {
		mark = styleImported.Render("✓ ")
	}
	date := "----------"
	if !f.CaptureTime.IsZero() {
		date = f.CaptureTime.Format("2006-01-02 15:04")
	}

	name := f.Name
	nameW := w - 2 - 5 - 9 - 17 - 2 - 4
	if nameW < 8 {
		nameW = 8
	}
	if len(name) > nameW {
		name = name[:nameW-1] + "…"
	}
	line := fmt.Sprintf("%s%s %-*s %8s  %s %s", check, badge, nameW, name, humanBytes(f.Size), styleDimText.Render(date), mark)
	if cursor {
		return styleRowCursor.MaxWidth(w).Render("▸" + line)
	}
	if m.hoverZone == rowID(f.Handle) {
		return styleRowHover.MaxWidth(w).Render(" " + line)
	}
	if m.imported[database.Key(f.Handle, f.Name, f.Size)] {
		return styleRowImported.MaxWidth(w).Render(" " + line)
	}
	return styleRow.MaxWidth(w).Render(" " + line)
}

func formatBadge(f ptpip.ObjectFormat) string {
	switch f {
	case ptpip.FormatJPEG:
		return styleBadgeJPG.Render("JPG")
	case ptpip.FormatNEF, ptpip.FormatUndefined:
		return styleBadgeNEF.Render("NEF")
	case ptpip.FormatMOV:
		return styleBadgeMOV.Render("MOV")
	default:
		return styleBadgeGen.Render(f.String())
	}
}

func (m Model) viewPreview(w, h int) string {
	var b strings.Builder
	enlarge := " ⤢ Enlarge "
	enlargeStyle := styleChip
	if m.hoverZone == enlargeID {
		enlargeStyle = styleChipHover
	}
	title := stylePaneTitle.Render(" Preview ")
	gap := w - lipgloss.Width(title) - lipgloss.Width(enlarge)
	if gap < 1 {
		gap = 1
	}
	b.WriteString(title + strings.Repeat(" ", gap) + enlargeStyle.Render(enlarge) + "\n")

	f, ok := m.currentFile()
	if !ok {
		b.WriteString(styleDimText.Render("\n  select a file"))
		return b.String()
	}

	imgRows := h - 8
	if imgRows < 4 {
		imgRows = 4
	}
	imgCols := w - 2

	b.WriteString(m.renderThumb(f.Handle, imgCols, imgRows))

	meta := [][2]string{
		{"Name", f.Name},
		{"Type", f.Format.String()},
		{"Size", humanBytes(f.Size)},
		{"Captured", formatTime(f)},
		{"Handle", fmt.Sprintf("0x%08X", f.Handle)},
	}
	if m.imported[database.Key(f.Handle, f.Name, f.Size)] {
		meta = append(meta, [2]string{"Status", "imported ✓"})
	} else {
		meta = append(meta, [2]string{"Status", "not imported"})
	}
	for _, kv := range meta {
		b.WriteString(fmt.Sprintf("%s %s\n", styleSubtle.Render(fmt.Sprintf("%9s", kv[0]+":")), kv[1]))
	}
	return b.String()
}

// renderThumb renders the thumbnail for handle into a cols x rows cell box,
// followed by the padding lines the layout needs. Inline protocols (Kitty,
// iTerm2) draw over the box with escape sequences, the half-block fallback
// produces real text lines, and a placeholder covers missing thumbnails and
// decode failures.
func (m Model) renderThumb(handle uint32, cols, rows int) string {
	if m.proto != thumbnail.ProtocolNone && m.thumbHandle == handle && len(m.thumbData) > 0 {
		if img := thumbnail.RenderInline(m.proto, m.thumbData, cols, rows); img != "" {
			if m.proto.Inline() {
				// The escape sequence occupies no layout lines itself.
				return img + strings.Repeat("\n", rows)
			}
			lines := strings.Count(img, "\n") + 1
			return img + strings.Repeat("\n", max(1, rows-lines+1))
		}
	}
	return thumbnail.Placeholder(cols, rows) + "\n"
}

func formatTime(f camera.File) string {
	if f.CaptureTime.IsZero() {
		return "unknown"
	}
	return f.CaptureTime.Format("2006-01-02 15:04:05")
}

func (m Model) viewBottomBar() string {
	if m.importing {
		ev := m.lastEv
		name := ev.File.Name
		if name == "" {
			name = "…"
		}
		pct := 0.0
		if ev.TotalBytes > 0 {
			pct = float64(ev.TotalBytesDone) / float64(ev.TotalBytes)
		}
		bar := m.prog.ViewAs(pct)
		eta := "--"
		if ev.ETA > 0 {
			eta = ev.ETA.String()
		}
		remaining := 0
		if ev.Count > 0 {
			remaining = ev.Count - ev.Index
		}
		line1 := fmt.Sprintf(" importing %s  (%d/%d)", styleAccent.Render(name), ev.Index, ev.Count)
		line2 := " " + bar
		line3 := styleStatusBar.Render(fmt.Sprintf("%s · %d left · eta %s · %s / %s · x cancel",
			humanSpeed(ev.Speed), remaining, eta, humanBytes(ev.TotalBytesDone), humanBytes(ev.TotalBytes)))
		return lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3)
	}

	free := ""
	for _, s := range m.storages {
		free = fmt.Sprintf("card: %s free of %s", humanBytes(int64(s.Free)), humanBytes(int64(s.Capacity)))
		break
	}
	status := styleStatusBar.Render(fmt.Sprintf("imported today: %d   %s", m.today, free))
	toast := ""
	if m.toast != "" {
		if m.toastErr {
			toast = styleToastErr.Render(m.toast)
		} else {
			toast = styleToastOK.Render(m.toast)
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, "", status, toast)
}

// ---- Overlays ----------------------------------------------------------

// renderOverlay draws box centered over base by overlaying it on a dimmed
// background (simple full replacement keeps rendering artifact-free).
func (m Model) renderOverlay(base, box string) string {
	_ = base
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) viewPreviewOverlay() string {
	f, ok := m.currentFile()
	if !ok {
		return styleOverlay.Render("no file selected")
	}
	w := m.width * 3 / 4
	h := m.height * 3 / 4
	var b strings.Builder
	b.WriteString(m.overlayHeader(f.Name, w-4) + "\n\n")
	b.WriteString(m.renderThumb(f.Handle, w-6, max(4, h-8)))
	b.WriteString(styleHelp.Render("click ✕ or outside to close · esc"))
	return styleOverlay.Width(w).Render(b.String())
}

func (m Model) viewDetailOverlay() string {
	header := m.overlayHeader("Metadata", m.detailView.Width) + "\n\n"
	return styleOverlay.Render(header + m.detailView.View() + "\n\n" + styleHelp.Render("↑/↓ scroll · click ✕ or outside to close"))
}

// overlayHeader renders an overlay title with a clickable ✕ close button pushed
// to the right edge. The close glyph highlights while hovered.
func (m Model) overlayHeader(title string, innerW int) string {
	t := styleTitle.Render(title)
	closeStyle := styleDimText
	if m.hoverZone == closeID {
		closeStyle = styleErrText.Bold(true)
	}
	x := closeStyle.Render("✕")
	gap := innerW - lipgloss.Width(t) - 1
	if gap < 1 {
		gap = 1
	}
	return t + strings.Repeat(" ", gap) + x
}

// viewCheatOverlay renders the keybindings cheatsheet toggled by the "?" button.
func (m Model) viewCheatOverlay() string {
	boxW := min(m.width-4, 52)
	var b strings.Builder
	b.WriteString(m.overlayHeader("Keyboard shortcuts (all optional)", boxW-4) + "\n\n")
	rows := [][2]string{
		{"↑/↓ k/j", "move cursor"},
		{"space", "toggle selection"},
		{"enter / dbl-click", "preview"},
		{"r", "refresh"},
		{"i / a", "import new / all"},
		{"S", "import selected"},
		{"f", "cycle filter"},
		{"P / D", "preview / details"},
		{"O", "open imported file"},
		{"s", "settings"},
		{"c / t", "switch camera / control"},
		{"w", "watch mode"},
		{"? / esc", "toggle this · close"},
	}
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("%s  %s\n", styleAccent.Render(fmt.Sprintf("%-18s", r[0])), styleSubtle.Render(r[1])))
	}
	b.WriteString("\n" + styleHint.Render("Tip: click anything — keys are just shortcuts."))
	return styleOverlay.Width(boxW).Render(b.String())
}

// detailContent builds the metadata text for the detail overlay.
func (m Model) detailContent() string {
	f, ok := m.currentFile()
	if !ok {
		return "no file selected"
	}
	var b strings.Builder
	kv := func(k, v string) { fmt.Fprintf(&b, "%-16s %s\n", k, v) }
	kv("Filename", f.Name)
	kv("Format", f.Format.String())
	kv("Size", fmt.Sprintf("%s (%d bytes)", humanBytes(f.Size), f.Size))
	kv("Captured", formatTime(f))
	kv("Object handle", fmt.Sprintf("0x%08X", f.Handle))
	kv("Storage ID", fmt.Sprintf("0x%08X", f.StorageID))
	dest := importer.DestPath(m.cfg.SaveFolder, f)
	kv("Destination", dest)
	if m.imported[database.Key(f.Handle, f.Name, f.Size)] {
		if p, err := m.db.DestPath(f.Handle, f.Name, f.Size); err == nil && p != "" {
			kv("Imported to", p)
		} else {
			kv("Imported", "yes")
		}
	} else {
		kv("Imported", "no")
	}
	info := m.cam.DeviceInfo()
	if info.Model != "" {
		b.WriteString("\n")
		kv("Camera", strings.TrimSpace(info.Manufacturer+" "+info.Model))
		kv("Serial", info.Serial)
		kv("Firmware", info.Version)
	}
	return b.String()
}

// ---- Settings ----------------------------------------------------------

func (m Model) viewSettings() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Settings") + "\n\n")

	label := func(i int, text string) string {
		if m.setFocus == i {
			return styleFieldFocus.Render("▸ " + text)
		}
		return styleFieldLabel.Render("  " + text)
	}
	toggle := func(on bool) string {
		if on {
			return styleImported.Render("[x] on")
		}
		return styleDimText.Render("[ ] off")
	}

	b.WriteString(label(0, "Save folder") + m.setInputs[0].View() + "\n")
	b.WriteString(label(1, "Camera IP") + m.setInputs[1].View() + "\n")
	b.WriteString(label(2, "Auto-import") + toggle(m.setAuto) + "\n")
	b.WriteString(label(3, "Open after import") + toggle(m.setOpen) + "\n\n")
	b.WriteString(styleHint.Render("click a field, toggle, or button · keys optional") + "\n\n")

	placed, lines := m.settingsToolbarLayout(0)
	b.WriteString(m.renderToolbar(placed, lines) + "\n\n")
	b.WriteString(styleHelp.Render("tab next · space toggle · enter save · esc cancel"))

	box := styleSettingsCard.Width(min(m.width-4, 80)).Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
