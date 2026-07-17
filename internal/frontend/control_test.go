package frontend

import (
	"testing"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
)

func choices(raws ...int64) []camera.Choice {
	out := make([]camera.Choice, len(raws))
	for i, r := range raws {
		out[i] = camera.Choice{Raw: r}
	}
	return out
}

func TestStepChoice(t *testing.T) {
	cs := choices(100, 200, 400, 800)

	tests := []struct {
		name    string
		current int64
		delta   int
		want    int64
		ok      bool
	}{
		{"step right", 200, 1, 400, true},
		{"step left", 200, -1, 100, true},
		{"clamp at end", 800, 1, 800, false},
		{"clamp at start", 100, -1, 100, false},
		{"nearest below", 250, 1, 400, true},
		{"nearest above", 350, -1, 200, true},
		{"zero delta", 200, 0, 200, false},
	}
	for _, tt := range tests {
		got, ok := stepChoice(cs, tt.current, tt.delta)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s: stepChoice(%d, %d) = (%d, %v), want (%d, %v)",
				tt.name, tt.current, tt.delta, got, ok, tt.want, tt.ok)
		}
	}
}

func TestStepChoiceEmpty(t *testing.T) {
	if got, ok := stepChoice(nil, 42, 1); got != 42 || ok {
		t.Fatalf("stepChoice(nil) = (%d, %v)", got, ok)
	}
}

func TestCtrlMoveCursorClamps(t *testing.T) {
	m := Model{height: 40}
	m.ctrlSettings = []camera.Setting{
		{Label: "a"}, {Label: "b"}, {Label: "c"},
	}
	m.ctrlMoveCursor(-5)
	if m.ctrlCursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.ctrlCursor)
	}
	m.ctrlMoveCursor(99)
	if m.ctrlCursor != m.ctrlTakeRow() {
		t.Fatalf("cursor = %d, want take row %d", m.ctrlCursor, m.ctrlTakeRow())
	}
}

func TestCtrlMoveCursorScrolls(t *testing.T) {
	m := Model{height: 14} // only 2 visible rows (14 - 12)
	for i := 0; i < 6; i++ {
		m.ctrlSettings = append(m.ctrlSettings, camera.Setting{Label: "s"})
	}
	vis := m.ctrlVisibleRows()
	for i := 0; i < 5; i++ {
		m.ctrlMoveCursor(1)
	}
	if m.ctrlCursor != 5 {
		t.Fatalf("cursor = %d, want 5", m.ctrlCursor)
	}
	if m.ctrlOffset != 5-vis+1 {
		t.Fatalf("offset = %d, want %d", m.ctrlOffset, 5-vis+1)
	}
	for i := 0; i < 5; i++ {
		m.ctrlMoveCursor(-1)
	}
	if m.ctrlCursor != 0 || m.ctrlOffset != 0 {
		t.Fatalf("cursor/offset = %d/%d, want 0/0", m.ctrlCursor, m.ctrlOffset)
	}
}

func TestPad(t *testing.T) {
	if got := pad("abc", 6); got != "abc   " {
		t.Errorf("pad short = %q", got)
	}
	if got := pad("abcdefgh", 5); got != "abcd…" {
		t.Errorf("pad long = %q", got)
	}
	if got := pad("abcde", 5); got != "abcde" {
		t.Errorf("pad exact = %q", got)
	}
}
