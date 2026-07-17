package frontend

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
)

// ---- Messages ----------------------------------------------------------

// ctrlSettingsMsg carries the settings read for the camera control overlay.
type ctrlSettingsMsg struct {
	settings []camera.Setting
	err      error
}

// ctrlSetMsg reports the outcome of writing one setting; on success it
// carries the re-read, confirmed setting.
type ctrlSetMsg struct {
	code    ptpip.DevicePropCode
	setting camera.Setting
	err     error
}

// ctrlCaptureMsg reports the outcome of a remote shutter release.
type ctrlCaptureMsg struct{ err error }

// ctrlRefreshMsg fires ~2s after a capture so the new photo shows up in the
// file list.
type ctrlRefreshMsg struct{}

// ---- Commands ----------------------------------------------------------

// readSettingsCmd reads every control-panel property from the camera.
func readSettingsCmd(cam *camera.Camera) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		settings, err := cam.ReadSettings(ctx)
		return ctrlSettingsMsg{settings: settings, err: err}
	}
}

// setSettingCmd writes a value and re-reads the property so the overlay shows
// the value the camera actually accepted.
func setSettingCmd(cam *camera.Camera, code ptpip.DevicePropCode, raw int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := cam.SetSetting(ctx, code, raw); err != nil {
			return ctrlSetMsg{code: code, err: err}
		}
		s, err := cam.ReadSetting(ctx, code)
		return ctrlSetMsg{code: code, setting: s, err: err}
	}
}

// captureCmd triggers the shutter.
func captureCmd(cam *camera.Camera) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return ctrlCaptureMsg{err: cam.TriggerCapture(ctx)}
	}
}

// captureRefreshCmd schedules a file-list refresh after a capture, giving the
// camera time to finish writing the new image.
func captureRefreshCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return ctrlRefreshMsg{} })
}

// ---- Row stepping ------------------------------------------------------

// stepChoice moves from the current raw value by delta positions within
// choices, clamping at the ends. When the current value is not an exact
// choice the nearest one is used as the starting point. The second return is
// false when no move is possible.
func stepChoice(choices []camera.Choice, current int64, delta int) (int64, bool) {
	if len(choices) == 0 || delta == 0 {
		return current, false
	}
	idx := 0
	best := int64(-1)
	for i, ch := range choices {
		d := ch.Raw - current
		if d < 0 {
			d = -d
		}
		if best < 0 || d < best {
			best = d
			idx = i
		}
	}
	next := idx + delta
	if next < 0 {
		next = 0
	}
	if next >= len(choices) {
		next = len(choices) - 1
	}
	if choices[next].Raw == current {
		return current, false
	}
	return choices[next].Raw, true
}

// ---- Update ------------------------------------------------------------

// openControl opens the camera control overlay and starts reading settings.
func (m Model) openControl() (tea.Model, tea.Cmd) {
	m.ctrlOverlay = true
	m.ctrlLoading = true
	m.ctrlPending = false
	m.ctrlCursor = 0
	m.ctrlOffset = 0
	return m, tea.Batch(m.spin.Tick, readSettingsCmd(m.cam))
}

// ctrlRowCount is the number of selectable rows: settings plus "Take photo".
func (m Model) ctrlRowCount() int { return len(m.ctrlSettings) + 1 }

// ctrlTakeRow is the index of the "Take photo" action row.
func (m Model) ctrlTakeRow() int { return len(m.ctrlSettings) }

// ctrlMoveCursor moves the control cursor and keeps it visible.
func (m *Model) ctrlMoveCursor(delta int) {
	m.ctrlCursor += delta
	if m.ctrlCursor < 0 {
		m.ctrlCursor = 0
	}
	if n := m.ctrlRowCount(); m.ctrlCursor >= n {
		m.ctrlCursor = n - 1
	}
	h := m.ctrlVisibleRows()
	if m.ctrlCursor < m.ctrlTakeRow() {
		if m.ctrlCursor < m.ctrlOffset {
			m.ctrlOffset = m.ctrlCursor
		}
		if m.ctrlCursor >= m.ctrlOffset+h {
			m.ctrlOffset = m.ctrlCursor - h + 1
		}
	}
	if m.ctrlOffset < 0 {
		m.ctrlOffset = 0
	}
}

// ctrlVisibleRows is how many setting rows fit in the overlay.
func (m Model) ctrlVisibleRows() int {
	h := m.height - 12
	if h < 3 {
		h = 3
	}
	if h > len(m.ctrlSettings) {
		h = len(m.ctrlSettings)
	}
	if h < 1 {
		h = 1
	}
	return h
}

// ctrlStep steps the selected setting by delta choices and sends the write.
func (m Model) ctrlStep(delta int) (tea.Model, tea.Cmd) {
	if m.ctrlPending || m.ctrlCursor >= len(m.ctrlSettings) {
		return m, nil
	}
	s := m.ctrlSettings[m.ctrlCursor]
	if !s.Writable {
		return m, nil
	}
	raw, ok := stepChoice(s.Choices, s.Raw, delta)
	if !ok {
		return m, nil
	}
	m.ctrlPending = true
	return m, setSettingCmd(m.cam, s.Code, raw)
}

// ctrlCapture triggers the shutter from the overlay.
func (m Model) ctrlCapture() (tea.Model, tea.Cmd) {
	return m, captureCmd(m.cam)
}

// controlKey handles keys while the camera control overlay is open.
func (m Model) controlKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "t":
		m.ctrlOverlay = false
		return m, nil
	case "up", "k":
		m.ctrlMoveCursor(-1)
		return m, nil
	case "down", "j":
		m.ctrlMoveCursor(1)
		return m, nil
	case "left", "h":
		return m.ctrlStep(-1)
	case "right", "l":
		return m.ctrlStep(1)
	case "enter":
		if m.ctrlCursor == m.ctrlTakeRow() {
			return m.ctrlCapture()
		}
		return m, nil
	case "T":
		return m.ctrlCapture()
	}
	return m, nil
}

// handleControlMsg routes the control-overlay messages. The second return is
// false when the message is not a control message.
func (m Model) handleControlMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case ctrlSettingsMsg:
		m.ctrlLoading = false
		m.ctrlSettings = msg.settings
		if m.ctrlCursor >= m.ctrlRowCount() {
			m.ctrlCursor = m.ctrlRowCount() - 1
		}
		if msg.err != nil && len(msg.settings) == 0 {
			mm, cmd := m.showToast("could not read settings: "+msg.err.Error(), true)
			return mm, cmd, true
		}
		return m, nil, true

	case ctrlSetMsg:
		m.ctrlPending = false
		if msg.err != nil {
			mm, cmd := m.showToast(msg.err.Error(), true)
			return mm, cmd, true
		}
		for i := range m.ctrlSettings {
			if m.ctrlSettings[i].Code == msg.code {
				m.ctrlSettings[i] = msg.setting
				break
			}
		}
		return m, nil, true

	case ctrlCaptureMsg:
		if msg.err != nil {
			mm, cmd := m.showToast(msg.err.Error(), true)
			return mm, cmd, true
		}
		mm, cmd := m.showToast("📸 Captured", false)
		return mm, tea.Batch(cmd, captureRefreshCmd()), true

	case ctrlRefreshMsg:
		if !m.refreshing && !m.importing {
			m.refreshing = true
			return m, refreshCmd(m.cam, m.db), true
		}
		return m, nil, true
	}
	return m, nil, false
}

// ---- Mouse -------------------------------------------------------------

// Fixed columns of a setting row, relative to the overlay content area:
// two cells of cursor marker, the label, "◀ ", the value, " ▶".
const (
	ctrlLabelW  = 16
	ctrlValueW  = 14
	ctrlDecCol  = 2 + ctrlLabelW                  // column of the ◀ arrow
	ctrlIncCol  = ctrlDecCol + 2 + ctrlValueW + 1 // column of the ▶ arrow
	ctrlRowsTop = 4                               // border + padding + title + blank line above the rows
)

// ctrlBoxOrigin returns the top-left screen cell of the rendered overlay,
// mirroring how lipgloss.Place centers a block (extra space split evenly,
// rounded down).
func (m Model) ctrlBoxOrigin() (x, y int) {
	box := m.viewControlOverlay()
	w, h := lipgloss.Width(box), lipgloss.Height(box)
	x = (m.width - w) / 2
	y = (m.height - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y
}

// controlMouse handles mouse input while the control overlay is open: wheel
// scrolls the rows, a click selects a row, clicks on the ◀/▶ zones step the
// value, and a click on "Take photo" fires the shutter.
func (m Model) controlMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Action == tea.MouseActionMotion:
		return m.setHover(m.ctrlHoverZone(msg.X, msg.Y)), nil

	case msg.Button == tea.MouseButtonWheelUp:
		m.ctrlMoveCursor(-1)
		return m, nil
	case msg.Button == tea.MouseButtonWheelDown:
		m.ctrlMoveCursor(1)
		return m, nil

	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		m.pressedZone = m.ctrlHoverZone(msg.X, msg.Y)
		return m, nil

	case msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft:
		m.pressedZone = ""
		return m.controlClick(msg)
	}
	return m, nil
}

// ctrlCloseZone reports whether (x, y) hits the control overlay's ✕ close glyph.
func (m Model) ctrlCloseZone(x, y int) bool {
	x0, y0 := m.ctrlBoxOrigin()
	box := m.viewControlOverlay()
	w := lipgloss.Width(box)
	// The ✕ sits at the last inner-text column: border (1) + right padding (2)
	// in from the box's right edge, i.e. x0+w-4. Accept the adjacent cell too.
	closeX := x0 + w - 4
	return y == y0+2 && (x == closeX || x == closeX+1)
}

// ctrlHoverZone returns the id of the interactive control-overlay element under
// (x, y).
func (m Model) ctrlHoverZone(x, y int) string {
	if m.ctrlCloseZone(x, y) {
		return closeID
	}
	x0, y0 := m.ctrlBoxOrigin()
	contentX := x0 + 3
	relY := msg2rel(y, y0)
	vis := m.ctrlVisibleRows()
	if relY >= 0 && relY < vis && len(m.ctrlSettings) > 0 {
		row := m.ctrlOffset + relY
		if row < len(m.ctrlSettings) {
			relX := x - contentX
			switch {
			case relX >= ctrlDecCol-1 && relX <= ctrlDecCol+1:
				return ctrlDecID(row)
			case relX >= ctrlIncCol-1 && relX <= ctrlIncCol+1:
				return ctrlIncID(row)
			}
		}
	}
	if relY == vis+1 {
		return ctrlTakeID
	}
	return ""
}

// msg2rel converts a screen row to a control-content-relative row.
func msg2rel(y, y0 int) int { return y - (y0 + ctrlRowsTop) }

// controlClick dispatches the action for the control-overlay element under the
// cursor on button release.
func (m Model) controlClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.ctrlCloseZone(msg.X, msg.Y) {
		m.ctrlOverlay = false
		m.hoverZone = ""
		return m, nil
	}
	x0, y0 := m.ctrlBoxOrigin()
	contentX := x0 + 3
	relY := msg2rel(msg.Y, y0)
	vis := m.ctrlVisibleRows()

	if relY >= 0 && relY < vis && len(m.ctrlSettings) > 0 {
		row := m.ctrlOffset + relY
		if row < len(m.ctrlSettings) {
			m.ctrlCursor = row
			relX := msg.X - contentX
			switch {
			case relX >= ctrlDecCol-1 && relX <= ctrlDecCol+1:
				return m.ctrlStep(-1)
			case relX >= ctrlIncCol-1 && relX <= ctrlIncCol+1:
				return m.ctrlStep(1)
			}
			return m, nil
		}
	}

	if relY == vis+1 {
		m.ctrlCursor = m.ctrlTakeRow()
		return m.ctrlCapture()
	}

	box := m.viewControlOverlay()
	w, h := lipgloss.Width(box), lipgloss.Height(box)
	if msg.X < x0 || msg.X >= x0+w || msg.Y < y0 || msg.Y >= y0+h {
		m.ctrlOverlay = false
	}
	return m, nil
}

// ---- View --------------------------------------------------------------

// ctrlNoSettingsText is shown when the body exposes none of the properties.
const ctrlNoSettingsText = "This camera does not expose remote settings over Wi-Fi"

// pad right-pads or truncates s to width display cells.
func pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w > width {
		r := []rune(s)
		for len(r) > 0 && lipgloss.Width(string(r)) > width-1 {
			r = r[:len(r)-1]
		}
		s = string(r) + "…"
		w = lipgloss.Width(s)
	}
	if w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

// viewControlOverlay renders the camera control card.
func (m Model) viewControlOverlay() string {
	var b strings.Builder
	b.WriteString(m.overlayHeader("Camera Control", ctrlIncCol+1) + "\n\n")

	switch {
	case m.ctrlLoading:
		b.WriteString(m.spin.View() + " reading camera settings…\n")
	case len(m.ctrlSettings) == 0:
		b.WriteString(styleDimText.Render(ctrlNoSettingsText) + "\n")
	default:
		vis := m.ctrlVisibleRows()
		end := m.ctrlOffset + vis
		if end > len(m.ctrlSettings) {
			end = len(m.ctrlSettings)
		}
		for i := m.ctrlOffset; i < end; i++ {
			b.WriteString(m.renderCtrlRow(i) + "\n")
		}
	}

	takeStyle := styleTakeBtn
	switch {
	case m.pressedZone == ctrlTakeID:
		takeStyle = styleTakeBtnPress
	case m.hoverZone == ctrlTakeID || m.ctrlCursor == m.ctrlTakeRow():
		takeStyle = styleTakeBtnHover
	}
	b.WriteString("\n  " + takeStyle.Render("◉ Take Photo") + "\n\n")
	b.WriteString(styleHelp.Render("click a value ◀ ▶ · ◉ take · ✕ close · keys optional"))

	// The widest content line is a setting row ending at ctrlIncCol (the ▶
	// column), i.e. ctrlIncCol+1 cells. styleOverlay pads two cells each side,
	// so add four to keep the text area wide enough that nothing wraps.
	w := ctrlIncCol + 1 + 4
	if max := m.width - 6; w > max && max > 20 {
		w = max
	}
	return styleOverlay.Width(w).Render(b.String())
}

// renderCtrlRow renders one setting row with fixed columns so the mouse
// arrow zones line up with the drawn arrows.
func (m Model) renderCtrlRow(i int) string {
	s := m.ctrlSettings[i]
	cursor := i == m.ctrlCursor

	mark := "  "
	if cursor {
		mark = "▸ "
	}
	value := s.Formatted
	if m.ctrlPending && cursor {
		value += " …"
	}

	if s.Writable && len(s.Choices) > 0 {
		dec := m.stepperGlyph("◀", ctrlDecID(i))
		inc := m.stepperGlyph("▶", ctrlIncID(i))
		label := mark + pad(s.Label, ctrlLabelW)
		val := pad(value, ctrlValueW)
		if cursor {
			label = styleRowCursor.Render(label)
			val = styleRowCursor.Render(val)
		} else {
			label = styleRow.Render(label)
			val = styleRow.Render(val)
		}
		return label + dec + " " + val + " " + inc
	}

	line := mark + pad(s.Label, ctrlLabelW) + "  " + pad(value, ctrlValueW)
	switch {
	case cursor:
		return styleRowCursor.Render(line)
	case !s.Writable:
		return styleDimText.Render(line)
	default:
		return styleRow.Render(line)
	}
}

// stepperGlyph renders a ◀/▶ stepper as a small clickable button, highlighted
// on hover and inverted while pressed.
func (m Model) stepperGlyph(glyph, id string) string {
	style := styleStepper
	switch {
	case m.pressedZone == id:
		style = styleStepperPress
	case m.hoverZone == id:
		style = styleStepperHover
	}
	return style.Render(glyph)
}

// Control-overlay hover/press zone ids.
const ctrlTakeID = "ctrl_take"

func ctrlDecID(i int) string { return "ctrl_dec_" + strconv.Itoa(i) }
func ctrlIncID(i int) string { return "ctrl_inc_" + strconv.Itoa(i) }
