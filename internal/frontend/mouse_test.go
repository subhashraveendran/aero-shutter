package frontend

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
	"github.com/subhashraveendran/aero-shutter/internal/config"
)

// testBrowserModel builds a minimal Model with n files in a fixed-size
// terminal, suitable for exercising the layout math.
func testBrowserModel(n, width, height int) Model {
	files := make([]camera.File, n)
	for i := range files {
		files[i] = camera.File{Handle: uint32(i + 1), Name: "DSC_0001.NEF"}
	}
	return Model{
		width:    width,
		height:   height,
		cam:      camera.New(),
		files:    files,
		selected: map[uint32]bool{},
		imported: map[string]bool{},
	}
}

func TestFileRowAt(t *testing.T) {
	m := testBrowserModel(50, 100, 30)
	m.offset = 10
	listW := m.listPaneWidth() // 55

	// First visible row sits below the top bar, pane border and title.
	if idx, ok := m.fileRowAt(5, listRowsTop); !ok || idx != 10 {
		t.Errorf("first row: got (%d, %v), want (10, true)", idx, ok)
	}
	// Third visible row maps offset+2.
	if idx, ok := m.fileRowAt(5, listRowsTop+2); !ok || idx != 12 {
		t.Errorf("third row: got (%d, %v), want (12, true)", idx, ok)
	}
	// The pane title line is not a file row.
	if _, ok := m.fileRowAt(5, listRowsTop-1); ok {
		t.Error("title line must not map to a file row")
	}
	// Clicks right of the list pane hit the preview pane.
	if _, ok := m.fileRowAt(listW, listRowsTop); ok {
		t.Error("preview pane click must not map to a file row")
	}
	// The left border column is not clickable.
	if _, ok := m.fileRowAt(0, listRowsTop); ok {
		t.Error("border column must not map to a file row")
	}
	// Rows past the end of the list do not map.
	m.offset = 48
	if _, ok := m.fileRowAt(5, listRowsTop+5); ok {
		t.Error("row past the last file must not map")
	}
	// Rows below the list area do not map.
	m.offset = 0
	if _, ok := m.fileRowAt(5, listRowsTop+m.listHeight()); ok {
		t.Error("row below the list area must not map")
	}
}

// TestToolbarHitZones verifies that the browser toolbar renders and hit-tests
// from the same placed-button geometry: every button's span maps back to its
// own id, gaps between chips are dead space, and clicking the toolbar dispatches
// the button's key.
func TestToolbarHitZones(t *testing.T) {
	m := testBrowserModel(50, 200, 30) // wide enough for a single toolbar row
	placed, lines := m.browserToolbarLayout()
	if lines < 1 {
		t.Fatalf("toolbar must occupy at least one line, got %d", lines)
	}
	if len(placed) != len(m.browserToolbar()) {
		t.Fatalf("placed %d buttons, want %d", len(placed), len(m.browserToolbar()))
	}
	// Every button resolves to itself across its whole span.
	for _, p := range placed {
		for x := p.startX; x <= p.endX; x++ {
			got, ok := toolbarZoneAt(placed, x, p.y)
			if !ok || got.id != p.id {
				t.Fatalf("button %q at x=%d: got (%q, %v)", p.id, x, got.id, ok)
			}
		}
	}
	// The single-cell gap between the first two chips is dead space.
	gapX := placed[0].endX + 1
	if _, ok := toolbarZoneAt(placed, gapX, placed[0].y); ok {
		t.Errorf("gap column %d must not be clickable", gapX)
	}
	// Off the toolbar row: nothing hit.
	if _, ok := toolbarZoneAt(placed, placed[0].startX, placed[0].y-1); ok {
		t.Error("clicks off the toolbar row must not hit")
	}
	// A press+release on the Quit button dispatches its "q" key (quit).
	var quit placedButton
	for _, p := range placed {
		if p.id == "tb_quit" {
			quit = p
		}
	}
	press := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: quit.startX, Y: quit.y}
	mm, _ := m.browserMouse(press)
	rel := tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: quit.startX, Y: quit.y}
	mm, _ = mm.(Model).browserMouse(rel)
	if !mm.(Model).quitting {
		t.Error("clicking the Quit button must quit")
	}
}

// TestToolbarWraps checks the toolbar wraps onto more lines on a narrow
// terminal, so the layout math never overflows the width.
func TestToolbarWraps(t *testing.T) {
	m := testBrowserModel(50, 30, 30) // narrow: buttons cannot fit on one row
	placed, lines := m.browserToolbarLayout()
	if lines < 2 {
		t.Fatalf("narrow toolbar should wrap, got %d lines", lines)
	}
	for _, p := range placed {
		if p.endX >= m.width && p.startX > 0 {
			t.Errorf("button %q overflows width %d: endX=%d", p.id, m.width, p.endX)
		}
	}
}

// TestFilterChipHitZones verifies the filter-chip row renders and hit-tests from
// the same geometry, and that clicking a chip selects that filter.
func TestFilterChipHitZones(t *testing.T) {
	m := testBrowserModel(50, 120, 30)
	chips := m.browserFilterChips()
	if len(chips) != len(listFilters) {
		t.Fatalf("got %d chips, want %d", len(chips), len(listFilters))
	}
	// Each chip resolves to its own index across its whole span.
	for _, c := range chips {
		for x := c.startX; x <= c.endX; x++ {
			got, ok := filterChipAt(chips, x, c.y)
			if !ok || got.idx != c.idx {
				t.Fatalf("chip %d at x=%d: got (%d, %v)", c.idx, x, got.idx, ok)
			}
		}
	}
	// The " · " separator between chips is dead space.
	sepX := chips[0].endX + 1
	if _, ok := filterChipAt(chips, sepX, chips[0].y); ok {
		t.Errorf("separator column %d must not be clickable", sepX)
	}
	// Clicking the last chip (index len-1) selects that filter.
	last := chips[len(chips)-1]
	press := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: last.startX, Y: last.y}
	mm, _ := m.browserMouse(press)
	rel := tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: last.startX, Y: last.y}
	mm, _ = mm.(Model).browserMouse(rel)
	if mm.(Model).filterIdx != last.idx {
		t.Errorf("clicking chip %d set filterIdx=%d", last.idx, mm.(Model).filterIdx)
	}
}

// TestToolbarHoverRerenders confirms mouse motion updates the hovered zone id so
// the render can highlight the button under the cursor.
func TestToolbarHoverRerenders(t *testing.T) {
	m := testBrowserModel(50, 200, 30)
	placed, _ := m.browserToolbarLayout()
	target := placed[0]
	motion := tea.MouseMsg{Action: tea.MouseActionMotion, X: target.startX, Y: target.y}
	mm, _ := m.browserMouse(motion)
	if mm.(Model).hoverZone != target.id {
		t.Errorf("hoverZone = %q, want %q", mm.(Model).hoverZone, target.id)
	}
	// Moving off every zone clears the hover.
	off := tea.MouseMsg{Action: tea.MouseActionMotion, X: 0, Y: 0}
	mm, _ = mm.(Model).browserMouse(off)
	if mm.(Model).hoverZone != "" {
		t.Errorf("hoverZone = %q after moving off, want empty", mm.(Model).hoverZone)
	}
}

// TestSettingsToolbarHitZones verifies the settings toolbar buttons render and
// hit-test from the same geometry.
func TestSettingsToolbarHitZones(t *testing.T) {
	m := testBrowserModel(0, 100, 30)
	placed, lines := m.settingsToolbarLayout(0)
	if lines != 1 {
		t.Fatalf("settings toolbar should fit one line, got %d", lines)
	}
	if len(placed) != len(m.settingsToolbar()) {
		t.Fatalf("placed %d buttons, want %d", len(placed), len(m.settingsToolbar()))
	}
	for _, p := range placed {
		got, ok := toolbarZoneAt(placed, p.startX, p.y)
		if !ok || got.id != p.id {
			t.Errorf("button %q at x=%d: got (%q, %v)", p.id, p.startX, got.id, ok)
		}
	}
	// shiftPlaced offsets every span so card-relative buttons map to screen
	// columns.
	shifted := shiftPlaced(placed, 10)
	if shifted[0].startX != placed[0].startX+10 {
		t.Errorf("shiftPlaced startX = %d, want %d", shifted[0].startX, placed[0].startX+10)
	}
}

// TestConnectToolbarHitZones verifies the connect toolbar dispatches its
// buttons' keys and that clicking Quit quits.
func TestConnectToolbarHitZones(t *testing.T) {
	m := New(config.Config{}, nil)
	m.width, m.height = 100, 30
	placed, _ := m.connectToolbarLayout()
	var quit placedButton
	for _, p := range placed {
		if p.id == "cn_quit" {
			quit = p
		}
	}
	rel := tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: quit.startX, Y: quit.y}
	mm, _ := m.connectMouse(rel)
	if !mm.(Model).quitting {
		t.Error("clicking connect Quit must quit")
	}
}

func TestScrollList(t *testing.T) {
	m := testBrowserModel(100, 100, 30)
	h := m.listHeight()

	m.scrollList(wheelScrollRows)
	if m.offset != 3 {
		t.Errorf("offset = %d, want 3", m.offset)
	}
	if m.cursor != m.offset {
		t.Errorf("cursor = %d, want dragged to %d", m.cursor, m.offset)
	}
	m.scrollList(-wheelScrollRows)
	if m.offset != 0 {
		t.Errorf("offset = %d, want 0", m.offset)
	}
	// Scrolling far past the end clamps to the last page.
	m.scrollList(1000)
	if m.offset != 100-h {
		t.Errorf("offset = %d, want %d", m.offset, 100-h)
	}
	m.scrollList(-1000)
	if m.offset != 0 || m.cursor < 0 {
		t.Errorf("offset = %d cursor = %d after scrolling to top", m.offset, m.cursor)
	}
	// Short lists never scroll.
	m = testBrowserModel(3, 100, 30)
	m.scrollList(wheelScrollRows)
	if m.offset != 0 {
		t.Errorf("short list offset = %d, want 0", m.offset)
	}
}

func TestPickerItemAt(t *testing.T) {
	// 3 items + 4 chrome lines = 7 content lines in a 21-row terminal:
	// gap 14, top pad 7, items start at row 7+2 = 9.
	if idx, ok := pickerItemAt(21, 3, 9); !ok || idx != 0 {
		t.Errorf("first item: got (%d, %v), want (0, true)", idx, ok)
	}
	if idx, ok := pickerItemAt(21, 3, 11); !ok || idx != 2 {
		t.Errorf("last item: got (%d, %v), want (2, true)", idx, ok)
	}
	if _, ok := pickerItemAt(21, 3, 8); ok {
		t.Error("blank line above the list must not map")
	}
	if _, ok := pickerItemAt(21, 3, 12); ok {
		t.Error("blank line below the list must not map")
	}
	// Terminal smaller than the content: top pad clamps to 0.
	if idx, ok := pickerItemAt(3, 3, 2); !ok || idx != 0 {
		t.Errorf("cramped terminal: got (%d, %v), want (0, true)", idx, ok)
	}
}

func TestBuildPicker(t *testing.T) {
	saved := []config.SavedCamera{
		{Name: "NIKON D5300", IP: "192.168.1.1", Serial: "111", LastSeen: time.Now()},
		{Name: "NIKON D750", IP: "192.168.1.9", Serial: "222", LastSeen: time.Now()},
	}
	found := []camera.Discovered{
		{Addr: "192.168.1.5", Model: "NIKON D5300", Serial: "111"}, // saved, moved address
		{Addr: "192.168.1.7", Model: "NIKON Z 6", Serial: "333"},   // new camera
	}
	items := buildPicker(saved, found)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	// Saved D5300 matched by serial: online, address updated.
	if !items[0].found || items[0].addr != "192.168.1.5" || !items[0].saved {
		t.Errorf("item 0 = %+v, want saved+found at new address", items[0])
	}
	// Saved D750 not found: listed but offline.
	if items[1].found || items[1].name != "NIKON D750" {
		t.Errorf("item 1 = %+v, want saved offline D750", items[1])
	}
	// New Z6 appended after the saved entries.
	if items[2].saved || !items[2].found || items[2].name != "NIKON Z 6" {
		t.Errorf("item 2 = %+v, want unsaved found Z6", items[2])
	}

	if got := firstAvailable(items); got != 0 {
		t.Errorf("firstAvailable = %d, want 0", got)
	}
	offline := []pickerItem{{found: false}, {found: true}}
	if got := firstAvailable(offline); got != 1 {
		t.Errorf("firstAvailable skips offline: got %d, want 1", got)
	}
	if got := firstAvailable([]pickerItem{{found: false}}); got != 0 {
		t.Errorf("firstAvailable with nothing online = %d, want 0", got)
	}
}

func TestBuildPickerMatchesByAddress(t *testing.T) {
	saved := []config.SavedCamera{{Name: "NIKON D5300", IP: "192.168.1.1"}}
	found := []camera.Discovered{{Addr: "192.168.1.1:15740", Model: "NIKON D5300"}}
	items := buildPicker(saved, found)
	if len(items) != 1 || !items[0].found {
		t.Fatalf("items = %+v, want one online entry matched by address", items)
	}
}
