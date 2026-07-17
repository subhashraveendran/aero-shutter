package frontend

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
	"github.com/subhashraveendran/aero-shutter/internal/config"
)

// pickerItem is one selectable camera on the connect screen.
type pickerItem struct {
	name   string // display name (model string)
	addr   string // address to connect to
	serial string // serial number, when known
	saved  bool   // present in the saved-camera list
	found  bool   // currently answering on the network
}

// buildPicker merges the saved camera list with live scan results: saved
// cameras come first (dimmed as offline when the scan did not find them),
// followed by newly discovered cameras. A saved entry matches a scan result
// by serial number first and address second.
func buildPicker(saved []config.SavedCamera, found []camera.Discovered) []pickerItem {
	used := make([]bool, len(found))
	items := make([]pickerItem, 0, len(saved)+len(found))
	for _, sc := range saved {
		it := pickerItem{name: sc.Name, addr: sc.IP, serial: sc.Serial, saved: true}
		for i, d := range found {
			if used[i] {
				continue
			}
			if (sc.Serial != "" && d.Serial == sc.Serial) || sameAddr(sc.IP, d.Addr) {
				used[i] = true
				it.found = true
				it.addr = d.Addr
				if d.Model != "" {
					it.name = d.Model
				}
				break
			}
		}
		if it.name == "" {
			it.name = "camera"
		}
		items = append(items, it)
	}
	for i, d := range found {
		if used[i] {
			continue
		}
		name := d.Model
		if name == "" {
			name = "PTP/IP camera"
		}
		items = append(items, pickerItem{name: name, addr: d.Addr, serial: d.Serial, found: true})
	}
	return items
}

// sameAddr reports whether two addresses point at the same camera, applying
// the default PTP/IP port before comparing.
func sameAddr(a, b string) bool {
	return camera.CanonicalAddr(a) == camera.CanonicalAddr(b)
}

// firstAvailable returns the index of the first currently reachable item,
// falling back to 0.
func firstAvailable(items []pickerItem) int {
	for i, it := range items {
		if it.found {
			return i
		}
	}
	return 0
}

// pickerExtraLines is the number of non-item lines in the picker content:
// title, blank line above the list, blank line below it, and the help line.
const pickerExtraLines = 4

// pickerTop returns the screen row of the picker's first content line, which
// mirrors how lipgloss.Place centers a block vertically (extra space split
// evenly, the top half rounded down).
func pickerTop(termHeight, itemCount int) int {
	gap := termHeight - (itemCount + pickerExtraLines)
	if gap <= 0 {
		return 0
	}
	return gap / 2
}

// pickerItemAt maps a screen row to a picker item index. Items start two
// lines below the picker top (title + blank line).
func pickerItemAt(termHeight, itemCount, y int) (int, bool) {
	idx := y - pickerTop(termHeight, itemCount) - 2
	if idx < 0 || idx >= itemCount {
		return 0, false
	}
	return idx, true
}

// viewPicker renders the camera selection list shown when more than one
// camera is available.
func (m Model) viewPicker() string {
	lines := []string{styleTitle.Render("Select camera"), ""}
	for i, it := range m.pickerItems {
		cursor := "  "
		if i == m.pickerCursor {
			cursor = styleAccent.Render("▸ ")
		}
		label := fmt.Sprintf("%s — %s", it.name, it.addr)
		switch {
		case it.saved && !it.found:
			label = styleDimText.Render(label + "  (saved, offline)")
		case it.saved:
			label = styleRow.Render(label + "  (saved)")
		default:
			label = styleRow.Render(label)
		}
		lines = append(lines, cursor+label)
	}
	lines = append(lines, "", styleHelp.Render("↑/↓ select · enter connect · d rescan · m manual ip · q quit"))
	content := strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
