package frontend

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// wheelScrollRows is how many file rows one wheel notch scrolls.
const wheelScrollRows = 3

// doubleClickWindow is the maximum delay between two clicks on the same row
// for them to count as a double-click.
const doubleClickWindow = 400 * time.Millisecond

// listRowsTop is the screen row of the first file row: the top bar, the pane
// border and the pane title line sit above it.
const listRowsTop = topBarHeight + 2

// hitZone maps an inclusive horizontal span of one screen line to the key it
// triggers when clicked.
type hitZone struct {
	startX, endX, y int
	key             string
}

// footerShortcuts lists the clickable help-bar labels and the keys they map
// to. The labels must match the substrings in helpText exactly.
var footerShortcuts = []struct{ label, key string }{
	{"q quit", "q"},
	{"r refresh", "r"},
	{"i import new", "i"},
	{"a import all", "a"},
	{"f filter", "f"},
	{"s settings", "s"},
	{"c camera", "c"},
}

// footerZones computes the clickable spans of the help bar from the same
// string used to render it. The help style pads one cell on the left, so
// every span is shifted right by one.
func footerZones(help string, y int) []hitZone {
	zones := make([]hitZone, 0, len(footerShortcuts))
	for _, s := range footerShortcuts {
		idx := strings.Index(help, s.label)
		if idx < 0 {
			continue
		}
		zones = append(zones, hitZone{
			startX: idx + 1,
			endX:   idx + len(s.label),
			y:      y,
			key:    s.key,
		})
	}
	return zones
}

// zoneAt returns the key of the zone containing (x, y).
func zoneAt(zones []hitZone, x, y int) (string, bool) {
	for _, z := range zones {
		if y == z.y && x >= z.startX && x <= z.endX {
			return z.key, true
		}
	}
	return "", false
}

// footerHitZones returns the clickable zones of the browser help bar, which
// occupies the bottom screen line.
func (m Model) footerHitZones() []hitZone {
	return footerZones(helpText, m.height-1)
}

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

// browserMouse handles mouse input on the browser screen: wheel scrolling
// over the file list, row clicks (select, toggle, double-click preview) and
// clicks on the help-bar shortcuts.
func (m Model) browserMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.previewOverlay || m.detailOverlay {
		// Wheel input over an overlay is ignored; a click closes it.
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.previewOverlay = false
			m.detailOverlay = false
		}
		return m, nil
	}

	switch {
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
		if key, ok := zoneAt(m.footerHitZones(), msg.X, msg.Y); ok {
			return m.browserKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		}
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
			if idx == m.cursor {
				if f, ok := m.currentFile(); ok {
					m.selected[f.Handle] = !m.selected[f.Handle]
				}
				return m, nil
			}
			m.cursor = idx
			m.clampCursor()
			return m, m.fetchCursorThumb()
		}
	}
	return m, nil
}

// connectMouse handles mouse input on the connect screen; only the camera
// picker reacts (wheel moves the cursor, a click connects to the item).
func (m Model) connectMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.picker {
		return m, nil
	}
	switch {
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
