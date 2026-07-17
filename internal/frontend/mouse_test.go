package frontend

import (
	"testing"
	"time"

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

func TestFooterZones(t *testing.T) {
	zones := footerZones(helpText, 29)
	if len(zones) != len(footerShortcuts) {
		t.Fatalf("got %d zones, want %d", len(zones), len(footerShortcuts))
	}
	// "q quit" leads the help text; the style pads one cell on the left.
	if key, ok := zoneAt(zones, 1, 29); !ok || key != "q" {
		t.Errorf("click at start of 'q quit': got (%q, %v), want (q, true)", key, ok)
	}
	if key, ok := zoneAt(zones, 6, 29); !ok || key != "q" {
		t.Errorf("click at end of 'q quit': got (%q, %v)", key, ok)
	}
	// The separator between labels is dead space.
	if _, ok := zoneAt(zones, 7, 29); ok {
		t.Error("separator must not be clickable")
	}
	// Wrong line: nothing hit.
	if _, ok := zoneAt(zones, 1, 28); ok {
		t.Error("clicks off the help line must not hit")
	}
	// Every declared shortcut resolves to its key at the zone start.
	for _, z := range zones {
		key, ok := zoneAt(zones, z.startX, z.y)
		if !ok || key != z.key {
			t.Errorf("zone %q at x=%d: got (%q, %v)", z.key, z.startX, key, ok)
		}
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
