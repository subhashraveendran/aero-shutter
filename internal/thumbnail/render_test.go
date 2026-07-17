package thumbnail

import (
	"strings"
	"testing"
)

func TestDetectProtocol(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Protocol
	}{
		{"kitty via window id", map[string]string{"KITTY_WINDOW_ID": "1"}, ProtocolKitty},
		{"kitty via TERM", map[string]string{"TERM": "xterm-kitty"}, ProtocolKitty},
		{"wezterm", map[string]string{"TERM_PROGRAM": "WezTerm"}, ProtocolKitty},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, ProtocolKitty},
		{"iterm2", map[string]string{"TERM_PROGRAM": "iTerm.app"}, ProtocolITerm2},
		{"plain xterm", map[string]string{"TERM": "xterm-256color"}, ProtocolHalfBlock},
		{"empty env", nil, ProtocolHalfBlock},
	}
	for _, tc := range cases {
		getenv := func(k string) string { return tc.env[k] }
		if got := detectProtocol(getenv); got != tc.want {
			t.Errorf("%s: detectProtocol = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	box := Placeholder(20, 5)
	lines := strings.Split(box, "\n")
	if len(lines) != 5 {
		t.Fatalf("placeholder has %d lines, want 5", len(lines))
	}
	if !strings.Contains(box, "No Preview") {
		t.Error("placeholder missing label")
	}
	if Placeholder(2, 1) != "[no preview]" {
		t.Error("tiny placeholder should degrade to plain text")
	}
}

func TestRenderInlineNone(t *testing.T) {
	if RenderInline(ProtocolNone, []byte{1}, 10, 5) != "" {
		t.Error("ProtocolNone must render nothing")
	}
	if RenderInline(ProtocolKitty, nil, 10, 5) != "" {
		t.Error("empty data must render nothing")
	}
}

func TestRenderITerm2(t *testing.T) {
	out := RenderInline(ProtocolITerm2, []byte{0xFF, 0xD8, 0xFF}, 12, 6)
	if !strings.HasPrefix(out, "\x1b]1337;File=") {
		t.Errorf("unexpected prefix: %q", out[:min(len(out), 20)])
	}
	if !strings.Contains(out, "width=12") || !strings.Contains(out, "height=6") {
		t.Error("missing cell dimensions")
	}
}

func TestLRUEviction(t *testing.T) {
	c := newLRU(2)
	c.put(1, []byte{1})
	c.put(2, []byte{2})
	c.get(1) // touch 1 so 2 becomes oldest
	c.put(3, []byte{3})
	if _, ok := c.get(2); ok {
		t.Error("least recently used entry not evicted")
	}
	if _, ok := c.get(1); !ok {
		t.Error("recently used entry evicted")
	}
	if _, ok := c.get(3); !ok {
		t.Error("new entry missing")
	}
}
