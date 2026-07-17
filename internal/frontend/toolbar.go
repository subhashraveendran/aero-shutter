package frontend

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/subhashraveendran/aero-shutter/internal/importer"
)

// This file is the single source of truth for the clickable toolbars and their
// hit zones. Buttons are rendered and hit-tested from the same []toolbarButton
// slice, so a click always lands on the button the user sees. Layout math for
// button widths and x-ranges lives here so render and hit-test never disagree.

// toolbarButton is one clickable chip in a toolbar.
type toolbarButton struct {
	id    string // stable zone id, also used as the hover/press key
	icon  string // leading glyph
	label string // text after the icon
	key   string // key action dispatched on click
}

// text returns the chip's inner text ("icon label" or just "label").
func (b toolbarButton) text() string {
	if b.icon == "" {
		return b.label
	}
	if b.label == "" {
		return b.icon
	}
	return b.icon + " " + b.label
}

// placedButton is a toolbarButton with its measured screen span, produced once
// per render so hit-testing reuses the exact same geometry.
type placedButton struct {
	toolbarButton
	startX, endX, y int
}

// browserToolbar returns the ordered buttons for the browser screen. The Filter
// button shows the active filter in its label and cycles it on click.
func (m Model) browserToolbar() []toolbarButton {
	return []toolbarButton{
		{id: "tb_refresh", icon: "⟳", label: "Refresh", key: "r"},
		{id: "tb_import_new", icon: "↓", label: "Import New", key: "i"},
		{id: "tb_import_all", icon: "⇊", label: "Import All", key: "a"},
		{id: "tb_select", icon: "⊕", label: "Select", key: " "},
		{id: "tb_filter", icon: "▤", label: "Filter: " + filterDisplay(listFilters[m.filterIdx]), key: "f"},
		{id: "tb_settings", icon: "‣", label: "Settings", key: "s"},
		{id: "tb_camera", icon: "▣", label: "Camera", key: "t"},
		{id: "tb_help", icon: "?", label: "", key: "?"},
		{id: "tb_quit", icon: "", label: "Quit", key: "q"},
	}
}

// Stable zone ids for the clickable elements outside the toolbars. These are
// shared between render (hover styling) and hit-test so the two never disagree.
const (
	enlargeID  = "prev_enlarge" // preview pane "⤢ Enlarge" affordance
	closeID    = "ov_close"     // overlay ✕ close button
	moreUpID   = "list_more_up"
	moreDownID = "list_more_down"
)

// rowID is the hover-zone id for the file row with the given handle.
func rowID(handle uint32) string {
	return "row_" + strconv.FormatUint(uint64(handle), 10)
}

// browserToolbarLayout places the browser toolbar buttons on the screen rows it
// occupies (immediately below the hint line). Render and hit-test both call
// this so the geometry is identical.
func (m Model) browserToolbarLayout() ([]placedButton, int) {
	_, lines := layoutToolbar(m.browserToolbar(), m.width, 0)
	y0 := m.height - lines
	if y0 < 0 {
		y0 = 0
	}
	placed, _ := layoutToolbar(m.browserToolbar(), m.width, y0)
	return placed, lines
}

// connectToolbarLayout places the connect toolbar buttons on the bottom screen
// rows.
func (m Model) connectToolbarLayout() ([]placedButton, int) {
	_, lines := layoutToolbar(m.connectToolbar(), m.width, 0)
	y0 := m.height - lines
	if y0 < 0 {
		y0 = 0
	}
	placed, _ := layoutToolbar(m.connectToolbar(), m.width, y0)
	return placed, lines
}

// settingsToolbarWidth is the wrapping width available to the settings toolbar
// inside the settings card (card width minus border and padding).
func (m Model) settingsToolbarWidth() int {
	return min(m.width-4, 80) - 6
}

// settingsToolbarLayout places the settings toolbar buttons at y0, the screen
// row of the toolbar within the rendered settings card. When y0 is 0 (render
// time) the buttons are positioned relative to the card content instead.
func (m Model) settingsToolbarLayout(y0 int) ([]placedButton, int) {
	placed, lines := layoutToolbar(m.settingsToolbar(), m.settingsToolbarWidth(), y0)
	return placed, lines
}

// connectToolbar returns the buttons for the connect screen. Automatic
// detection is the primary path, so the manual-IP affordance is an
// understated "Advanced" button; it becomes a Cancel/Connect pair only once
// the user has opened the manual field.
func (m Model) connectToolbar() []toolbarButton {
	btns := []toolbarButton{
		{id: "cn_detect", icon: "⟳", label: "Rescan", key: "d"},
	}
	if m.ipInput.Focused() {
		btns = append(btns,
			toolbarButton{id: "cn_connect", icon: "‣", label: "Connect", key: "enter"},
			toolbarButton{id: "cn_ip", icon: "✕", label: "Cancel", key: "esc"},
		)
	} else {
		btns = append(btns,
			toolbarButton{id: "cn_ip", icon: "‣", label: "Advanced / Enter IP", key: "tab"},
		)
	}
	return append(btns, toolbarButton{id: "cn_quit", icon: "", label: "Quit", key: "q"})
}

// settingsToolbar returns the buttons for the settings screen.
func (m Model) settingsToolbar() []toolbarButton {
	return []toolbarButton{
		{id: "st_save", icon: "✔", label: "Save & Back", key: "enter"},
		{id: "st_cancel", icon: "✕", label: "Cancel", key: "esc"},
	}
}

// layoutToolbar measures the buttons into a single row of chips, wrapping onto
// additional lines when the terminal is too narrow. y0 is the screen row of the
// first toolbar line. The returned placed buttons carry exact x-ranges.
func layoutToolbar(btns []toolbarButton, width, y0 int) ([]placedButton, int) {
	const gap = 1     // space between chips
	const chipPad = 2 // one padding cell each side (matches styleTbBtn)
	placed := make([]placedButton, 0, len(btns))
	x, y := 0, y0
	lines := 1
	for _, b := range btns {
		chipW := lipgloss.Width(b.text()) + chipPad
		if x > 0 && x+chipW > width {
			// Wrap to the next line.
			x = 0
			y++
			lines++
		}
		placed = append(placed, placedButton{
			toolbarButton: b,
			startX:        x,
			endX:          x + chipW - 1,
			y:             y,
		})
		x += chipW + gap
	}
	return placed, lines
}

// renderToolbar renders the placed buttons into their rows, highlighting the
// hovered button and inverting the pressed one. The output aligns cell-for-cell
// with the spans in placed, so clicks map back exactly.
func (m Model) renderToolbar(placed []placedButton, lines int) string {
	rows := make([][]string, lines)
	y0 := 0
	if len(placed) > 0 {
		y0 = placed[0].y
	}
	// Track the current x per line to insert inter-chip gaps.
	lineX := make([]int, lines)
	for _, p := range placed {
		li := p.y - y0
		if li < 0 || li >= lines {
			continue
		}
		if pad := p.startX - lineX[li]; pad > 0 {
			rows[li] = append(rows[li], strings.Repeat(" ", pad))
		}
		rows[li] = append(rows[li], m.renderChip(p.toolbarButton))
		lineX[li] = p.endX + 1
	}
	out := make([]string, lines)
	for i, r := range rows {
		out[i] = strings.Join(r, "")
	}
	return strings.Join(out, "\n")
}

// renderChip renders one button chip in its hover/press state.
func (m Model) renderChip(b toolbarButton) string {
	style := styleTbBtn
	switch {
	case m.pressedZone == b.id:
		style = styleTbBtnPress
	case m.hoverZone == b.id:
		style = styleTbBtnHover
	}
	return style.Render(b.text())
}

// toolbarZoneAt returns the button whose span contains (x, y).
func toolbarZoneAt(placed []placedButton, x, y int) (placedButton, bool) {
	for _, p := range placed {
		if y == p.y && x >= p.startX && x <= p.endX {
			return p, true
		}
	}
	return placedButton{}, false
}

// ---- Filter chips ------------------------------------------------------

// filterChip is one clickable filter selector at the top of the list pane.
type filterChip struct {
	idx             int // index into listFilters
	label           string
	startX, endX, y int
}

// filterDisplay maps a filter to its title-cased chip/button label.
func filterDisplay(f importer.Filter) string {
	switch f {
	case importer.FilterAll:
		return "All"
	case importer.FilterNew:
		return "New"
	case importer.FilterRAW:
		return "RAW"
	case importer.FilterJPEG:
		return "JPEG"
	case importer.FilterImported:
		return "Imported"
	default:
		return f.String()
	}
}

// filterChipLabels are the display labels for the chip row, matching the order
// of listFilters.
func filterChipLabels() []string {
	labels := make([]string, len(listFilters))
	for i, f := range listFilters {
		labels[i] = filterDisplay(f)
	}
	return labels
}

// layoutFilterChips measures the filter chips onto the pane's chip row. x0 is
// the left screen column of the pane's inner content; y is the chip row. Chips
// pad one cell each side (matching styleChip) and are separated by " · ".
func layoutFilterChips(x0, y int) []filterChip {
	labels := filterChipLabels()
	chips := make([]filterChip, 0, len(labels))
	const sep = 3 // " · "
	x := x0
	for i, label := range labels {
		w := lipgloss.Width(label) + 2 // one padding cell each side
		chips = append(chips, filterChip{
			idx:    i,
			label:  label,
			startX: x,
			endX:   x + w - 1,
			y:      y,
		})
		x += w + sep
	}
	return chips
}

// renderFilterChips renders the chip row, highlighting the active filter and
// the hovered chip.
func (m Model) renderFilterChips() string {
	labels := filterChipLabels()
	parts := make([]string, 0, len(labels)*2)
	for i, label := range labels {
		style := styleChip
		switch {
		case i == m.filterIdx:
			style = styleChipActive
		case m.hoverZone == filterChipID(i):
			style = styleChipHover
		}
		if i > 0 {
			parts = append(parts, styleDimText.Render(" · "))
		}
		parts = append(parts, style.Render(label))
	}
	return strings.Join(parts, "")
}

// filterChipID is the hover-zone id for filter chip i.
func filterChipID(i int) string {
	return "chip_" + listFilters[i].String()
}

// filterChipAt returns the filter chip whose span contains (x, y).
func filterChipAt(chips []filterChip, x, y int) (filterChip, bool) {
	for _, c := range chips {
		if y == c.y && x >= c.startX && x <= c.endX {
			return c, true
		}
	}
	return filterChip{}, false
}
