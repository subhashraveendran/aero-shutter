package frontend

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// wheelScrollRows is how many file rows one wheel notch scrolls.
const wheelScrollRows = 3

// doubleClickWindow is the maximum delay between two clicks on the same row
// for them to count as a double-click.
const doubleClickWindow = 400 * time.Millisecond

// Screen rows of the fixed elements at the top of the browser's list pane.
// Top bar (row 0), pane border (row 1), title (row 2), filter chips (row 3),
// top "more" indicator (row 4), then the first file row.
const (
	filterChipRow = topBarHeight + 2 // screen y of the filter-chip row
	listMoreUpRow = topBarHeight + 3 // screen y of the "▲ more" indicator
	listRowsTop   = topBarHeight + 4 // screen y of the first file row
)

// listPaneWidth is the total width of the left (file list) pane including its
// border, mirroring the split used by viewBody.
func (m Model) listPaneWidth() int {
	listW := m.width * 55 / 100
	if listW < 30 {
		listW = min(30, m.width-2)
	}
	return listW
}

// fileRowAt maps a click position to an index into visibleFiles, accounting
// for the top bar, the pane border and title, and the scroll offset.
func (m Model) fileRowAt(x, y int) (int, bool) {
	listW := m.listPaneWidth()
	if x < 1 || x > listW-2 {
		return 0, false
	}
	row := y - listRowsTop
	if row < 0 || row >= m.listHeight() {
		return 0, false
	}
	idx := m.offset + row
	if idx >= len(m.visibleFiles()) {
		return 0, false
	}
	return idx, true
}

// scrollList moves the list viewport by delta rows, dragging the cursor along
// so it stays inside the visible window.
func (m *Model) scrollList(delta int) {
	n := len(m.visibleFiles())
	h := m.listHeight()
	maxOffset := n - h
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.offset += delta
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
	if m.cursor < m.offset {
		m.cursor = m.offset
	}
	if m.cursor > m.offset+h-1 {
		m.cursor = m.offset + h - 1
	}
	if m.cursor > n-1 {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// filterChipX0 is the screen column of the first filter chip: the pane border
// occupies column 0, so the pane's inner content starts at column 1.
const filterChipX0 = 1

// browserFilterChips returns the placed filter chips for the current layout.
func (m Model) browserFilterChips() []filterChip {
	return layoutFilterChips(filterChipX0, filterChipRow)
}

// setHover updates the hovered zone id for id, re-rendering only when it
// changes. It returns the (possibly updated) model.
func (m Model) setHover(id string) Model {
	if m.hoverZone != id {
		m.hoverZone = id
	}
	return m
}

// browserHoverZone returns the id of the zone under (x, y) on the browser
// screen, or "" when nothing interactive is there.
func (m Model) browserHoverZone(x, y int) string {
	placed, _ := m.browserToolbarLayout()
	if p, ok := toolbarZoneAt(placed, x, y); ok {
		return p.id
	}
	if c, ok := filterChipAt(m.browserFilterChips(), x, y); ok {
		return filterChipID(c.idx)
	}
	if id, ok := m.enlargeZone(x, y); ok {
		return id
	}
	if m.offset > 0 && y == listMoreUpRow && x >= 1 && x <= m.listPaneWidth()-2 {
		return moreUpID
	}
	if m.hasMoreBelow() && y == m.listMoreDownRow() && x >= 1 && x <= m.listPaneWidth()-2 {
		return moreDownID
	}
	if idx, ok := m.fileRowAt(x, y); ok {
		files := m.visibleFiles()
		if idx >= 0 && idx < len(files) {
			return rowID(files[idx].Handle)
		}
	}
	return ""
}

// enlargeZone reports whether (x, y) is on the preview pane's "⤢ Enlarge"
// affordance, which sits on the preview title row at the pane's right edge.
func (m Model) enlargeZone(x, y int) (string, bool) {
	// The preview pane title sits below the top bar and the pane's top border.
	if y != topBarHeight+2 {
		return "", false
	}
	const labelW = 11 // display width of " ⤢ Enlarge " (padding + glyph + text)
	// The chip is right-aligned inside the preview pane's right border.
	right := m.width - 2
	left := right - labelW + 1
	if x >= left && x <= right {
		return enlargeID, true
	}
	return "", false
}

// hasMoreBelow reports whether the file list overflows past the last visible
// row.
func (m Model) hasMoreBelow() bool {
	visible := m.listHeight()
	return m.offset+visible < len(m.visibleFiles())
}

// listMoreDownRow is the screen row of the bottom "▼ more" indicator.
func (m Model) listMoreDownRow() int {
	return listRowsTop + m.listHeight()
}

// checkboxZone reports whether (x, y) falls on a file row's ☐/☑ checkbox glyph
// (the two leading cells of the row content) and returns the row index.
func (m Model) checkboxZone(x, y int) (int, bool) {
	idx, ok := m.fileRowAt(x, y)
	if !ok {
		return 0, false
	}
	// The checkbox occupies the first two content cells (screen x 1..2), after
	// the pane border. fileRowAt already bounded x to the pane interior.
	if x >= 1 && x <= 2 {
		return idx, true
	}
	return 0, false
}

// browserMouse handles mouse input on the browser screen: hover feedback, wheel
// scrolling over the file list, row clicks (select, checkbox toggle,
// double-click preview), filter chips, the preview "Enlarge" affordance, the
// overflow indicators and the toolbar buttons.
func (m Model) browserMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.ctrlOverlay {
		return m.controlMouse(msg)
	}
	if m.cheatOverlay {
		return m.overlayMouse(msg, func(mm *Model) { mm.cheatOverlay = false })
	}
	if m.previewOverlay {
		return m.overlayMouse(msg, func(mm *Model) { mm.previewOverlay = false })
	}
	if m.detailOverlay {
		return m.overlayMouse(msg, func(mm *Model) { mm.detailOverlay = false })
	}

	switch {
	case msg.Action == tea.MouseActionMotion:
		return m.setHover(m.browserHoverZone(msg.X, msg.Y)), nil

	case msg.Button == tea.MouseButtonWheelUp, msg.Button == tea.MouseButtonWheelDown:
		if msg.X >= m.listPaneWidth() {
			return m, nil // wheel over the preview pane does nothing
		}
		delta := wheelScrollRows
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -wheelScrollRows
		}
		m.scrollList(delta)
		return m, m.fetchCursorThumb()

	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		m.pressedZone = m.browserHoverZone(msg.X, msg.Y)
		return m, nil

	case msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft:
		return m.browserClick(msg)
	}
	return m, nil
}

// browserClick dispatches the action for the zone under the cursor on button
// release, matching the pressed zone so a drag off a button cancels it.
func (m Model) browserClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.pressedZone = ""

	// Toolbar buttons dispatch the same action as their key.
	placed, _ := m.browserToolbarLayout()
	if p, ok := toolbarZoneAt(placed, msg.X, msg.Y); ok {
		return m.browserKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(p.key)})
	}

	// Filter chips set the filter directly.
	if c, ok := filterChipAt(m.browserFilterChips(), msg.X, msg.Y); ok {
		m.setFilter(c.idx)
		return m, m.fetchCursorThumb()
	}

	// "Enlarge" opens the big preview.
	if _, ok := m.enlargeZone(msg.X, msg.Y); ok {
		m.previewOverlay = true
		return m, m.fetchCursorThumb()
	}

	// Overflow indicators page the list.
	if m.offset > 0 && msg.Y == listMoreUpRow && msg.X >= 1 && msg.X <= m.listPaneWidth()-2 {
		m.scrollList(-m.listHeight())
		return m, m.fetchCursorThumb()
	}
	if m.hasMoreBelow() && msg.Y == m.listMoreDownRow() && msg.X >= 1 && msg.X <= m.listPaneWidth()-2 {
		m.scrollList(m.listHeight())
		return m, m.fetchCursorThumb()
	}

	// Checkbox toggles selection without moving the cursor.
	if idx, ok := m.checkboxZone(msg.X, msg.Y); ok {
		files := m.visibleFiles()
		if idx >= 0 && idx < len(files) {
			h := files[idx].Handle
			m.selected[h] = !m.selected[h]
		}
		return m, nil
	}

	// File rows: single click selects, double click previews.
	if idx, ok := m.fileRowAt(msg.X, msg.Y); ok {
		now := time.Now()
		isDouble := idx == m.lastClickRow && now.Sub(m.lastClickTime) <= doubleClickWindow
		m.lastClickRow, m.lastClickTime = idx, now
		if isDouble {
			m.cursor = idx
			m.clampCursor()
			m.previewOverlay = true
			return m, m.fetchCursorThumb()
		}
		m.cursor = idx
		m.clampCursor()
		return m, m.fetchCursorThumb()
	}
	return m, nil
}

// overlayMouse handles hover + click for a simple overlay: hovering the ✕ close
// glyph highlights it, and a click on ✕ or outside the box closes it via close.
func (m Model) overlayMouse(msg tea.MouseMsg, close func(*Model)) (tea.Model, tea.Cmd) {
	switch {
	case msg.Action == tea.MouseActionMotion:
		if m.overlayCloseZone(msg.X, msg.Y) {
			return m.setHover(closeID), nil
		}
		return m.setHover(""), nil
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if m.overlayCloseZone(msg.X, msg.Y) || m.overlayOutside(msg.X, msg.Y) {
			close(&m)
			m.hoverZone = ""
		}
		return m, nil
	}
	return m, nil
}

// settingsCardOrigin returns the top-left screen cell of the centered settings
// card, mirroring lipgloss.Place.
func (m Model) settingsCardOrigin() (x, y int) {
	box := m.viewSettings()
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

// Content row offsets inside the settings card, relative to the first content
// line (the "Settings" title).
const (
	setFieldSaveRow  = 2 // "Save folder" input row
	setFieldIPRow    = 3 // "Camera IP" input row
	setToggleAutoRow = 4 // "Auto-import" toggle row
	setToggleOpenRow = 5 // "Open after import" toggle row
	setToolbarRow    = 9 // first toolbar line
)

// settingsMouse handles hover and clicks on the settings card: clicking a field
// focuses it, clicking a toggle flips it, and the toolbar buttons dispatch their
// key.
func (m Model) settingsMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	x0, y0 := m.settingsCardOrigin()
	contentY := y0 + 2 // border + padding-top
	contentX := x0 + 3 // border + padding-left (2)
	placed, _ := m.settingsToolbarLayout(contentY + setToolbarRow)

	switch {
	case msg.Action == tea.MouseActionMotion:
		id := ""
		// Shift toolbar spans by the content-left offset for hit-testing.
		if p, ok := toolbarZoneAt(shiftPlaced(placed, contentX), msg.X, msg.Y); ok {
			id = p.id
		}
		return m.setHover(id), nil

	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if p, ok := toolbarZoneAt(shiftPlaced(placed, contentX), msg.X, msg.Y); ok {
			m.pressedZone = p.id
		}
		return m, nil

	case msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft:
		m.pressedZone = ""
		if p, ok := toolbarZoneAt(shiftPlaced(placed, contentX), msg.X, msg.Y); ok {
			switch p.key {
			case "enter":
				return m.saveSettings()
			case "esc":
				m.screen = screenBrowser
				m.blurSettings()
				return m, nil
			}
		}
		// Field / toggle rows.
		row := msg.Y - contentY
		switch row {
		case setFieldSaveRow:
			return m.focusSettings(0)
		case setFieldIPRow:
			return m.focusSettings(1)
		case setToggleAutoRow:
			mm, _ := m.focusSettings(2)
			model := mm.(Model)
			model.setAuto = !model.setAuto
			return model, nil
		case setToggleOpenRow:
			mm, _ := m.focusSettings(3)
			model := mm.(Model)
			model.setOpen = !model.setOpen
			return model, nil
		}
	}
	return m, nil
}

// shiftPlaced returns a copy of placed with every span shifted right by dx, so
// buttons laid out relative to a card's content align with screen columns.
func shiftPlaced(placed []placedButton, dx int) []placedButton {
	out := make([]placedButton, len(placed))
	for i, p := range placed {
		p.startX += dx
		p.endX += dx
		out[i] = p
	}
	return out
}

// currentOverlayBox returns the rendered box for whichever simple overlay is
// open, so its centered origin can be computed for hit-testing.
func (m Model) currentOverlayBox() string {
	switch {
	case m.cheatOverlay:
		return m.viewCheatOverlay()
	case m.previewOverlay:
		return m.viewPreviewOverlay()
	case m.detailOverlay:
		return m.viewDetailOverlay()
	}
	return ""
}

// overlayOrigin returns the top-left screen cell of the centered overlay box,
// mirroring lipgloss.Place (extra space split evenly, rounded down).
func (m Model) overlayOrigin() (x, y, w, h int) {
	box := m.currentOverlayBox()
	w, h = lipgloss.Width(box), lipgloss.Height(box)
	x = (m.width - w) / 2
	y = (m.height - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y, w, h
}

// overlayCloseZone reports whether (x, y) is on the overlay's ✕ close glyph,
// which sits on the header row at the box's right edge (inside its border and
// padding).
func (m Model) overlayCloseZone(x, y int) bool {
	x0, y0, w, _ := m.overlayOrigin()
	// Header row: border (1) + padding-top (1) inside the box.
	headerY := y0 + 2
	// The ✕ sits at the last inner-text column: border (1) + right padding (2)
	// in from the box's right edge, i.e. x0+w-4.
	closeX := x0 + w - 4
	return y == headerY && (x == closeX || x == closeX+1)
}

// overlayOutside reports whether (x, y) is outside the overlay box entirely.
func (m Model) overlayOutside(x, y int) bool {
	x0, y0, w, h := m.overlayOrigin()
	return x < x0 || x >= x0+w || y < y0 || y >= y0+h
}

// connectMouse handles mouse input on the connect screen: the bottom toolbar
// (hover + click), and the camera picker (wheel moves the cursor, a click
// connects to the item).
func (m Model) connectMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.picker {
		switch {
		case msg.Action == tea.MouseActionMotion:
			id := ""
			if idx, ok := pickerItemAt(m.height, len(m.pickerItems), msg.Y); ok {
				id = pickerItemID(idx)
			}
			return m.setHover(id), nil
		case msg.Button == tea.MouseButtonWheelUp:
			if m.pickerCursor > 0 {
				m.pickerCursor--
			}
		case msg.Button == tea.MouseButtonWheelDown:
			if m.pickerCursor < len(m.pickerItems)-1 {
				m.pickerCursor++
			}
		case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
			if idx, ok := pickerItemAt(m.height, len(m.pickerItems), msg.Y); ok {
				m.pickerCursor = idx
				return m.pickerConnect()
			}
		}
		return m, nil
	}

	placed, _ := m.connectToolbarLayout()
	switch {
	case msg.Action == tea.MouseActionMotion:
		id := ""
		if p, ok := toolbarZoneAt(placed, msg.X, msg.Y); ok {
			id = p.id
		}
		return m.setHover(id), nil
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if p, ok := toolbarZoneAt(placed, msg.X, msg.Y); ok {
			m.pressedZone = p.id
		}
		return m, nil
	case msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft:
		m.pressedZone = ""
		if p, ok := toolbarZoneAt(placed, msg.X, msg.Y); ok {
			return m.dispatchConnectKey(p.key)
		}
	}
	return m, nil
}

// dispatchConnectKey routes a connect-toolbar button's key through the connect
// key handler, so a click and its shortcut behave identically.
func (m Model) dispatchConnectKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab", "m", "i":
		return m, m.ipInput.Focus()
	case "esc":
		m.ipInput.Blur()
		return m, nil
	case "enter":
		return m.updateConnect(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m.updateConnect(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}
