package thumbnail

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// smallJPEG returns a valid JPEG of the given pixel size for render tests.
func smallJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

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
		{"vscode", map[string]string{"TERM_PROGRAM": "vscode"}, ProtocolITerm2},
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

func TestProtocolFromMode(t *testing.T) {
	vscodeEnv := func(k string) string {
		if k == "TERM_PROGRAM" {
			return "vscode"
		}
		return ""
	}
	cases := []struct {
		mode string
		want Protocol
	}{
		{"kitty", ProtocolKitty},
		{"iterm2", ProtocolITerm2},
		{"halfblock", ProtocolHalfBlock},
		{" HalfBlock ", ProtocolHalfBlock},
		{"auto", ProtocolITerm2}, // falls through to detection (vscode env)
		{"", ProtocolITerm2},
		{"garbage", ProtocolITerm2},
	}
	for _, tc := range cases {
		if got := protocolFromMode(tc.mode, vscodeEnv); got != tc.want {
			t.Errorf("protocolFromMode(%q) = %v, want %v", tc.mode, got, tc.want)
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

func TestLabeledBox(t *testing.T) {
	box := LabeledBox("Loading preview…", 24, 6)
	lines := strings.Split(box, "\n")
	if len(lines) != 6 {
		t.Fatalf("box has %d lines, want 6", len(lines))
	}
	for i, ln := range lines {
		if w := len([]rune(ln)); w != 24 {
			t.Errorf("line %d width = %d, want 24", i, w)
		}
	}
	if !strings.Contains(box, "Loading preview…") {
		t.Error("box missing label")
	}
	// Label should be roughly centered: leading spaces on the label line > 0.
	var labelLine string
	for _, ln := range lines {
		if strings.Contains(ln, "Loading") {
			labelLine = ln
		}
	}
	inner := strings.Trim(labelLine, "│")
	left := len(inner) - len(strings.TrimLeft(inner, " "))
	right := len(inner) - len(strings.TrimRight(inner, " "))
	if left == 0 || right == 0 || left-right > 1 || right-left > 1 {
		t.Errorf("label not centered: left=%d right=%d", left, right)
	}
	// Placeholder is a LabeledBox with the "No Preview" label.
	if !strings.Contains(Placeholder(20, 5), "No Preview") {
		t.Error("Placeholder should carry the No Preview label")
	}
	if LabeledBox("x", 2, 1) != "[x]" {
		t.Error("tiny box should degrade to plain text")
	}
}

func TestScaleToImageUpscalesMidpoint(t *testing.T) {
	// 2x2 checkerboard upscaled to 3x3: the center pixel sits between all four
	// sources so it must interpolate to mid-gray; corners map onto sources.
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{0, 0, 0, 255})
	img.Set(1, 0, color.RGBA{255, 255, 255, 255})
	img.Set(0, 1, color.RGBA{255, 255, 255, 255})
	img.Set(1, 1, color.RGBA{0, 0, 0, 255})

	out := scaleToImage(img, 3, 3)
	if b := out.Bounds(); b.Dx() != 3 || b.Dy() != 3 {
		t.Fatalf("bounds = %v, want 3x3", b)
	}
	r, g, bl, a := out.At(1, 1).RGBA()
	for _, ch := range []uint32{r >> 8, g >> 8, bl >> 8} {
		if ch < 126 || ch > 129 {
			t.Errorf("center channel = %d, want ~127", ch)
		}
	}
	if a>>8 != 0xFF {
		t.Errorf("alpha = %d, want opaque", a>>8)
	}
}

func TestUpscaleTarget(t *testing.T) {
	// A tiny 160x120 thumb shown in a big box should upscale (preserving aspect
	// ratio), not downscale.
	w, h := upscaleTarget(160, 120, 60, 30)
	if w <= 160 || h <= 120 {
		t.Errorf("small source in big box should upscale, got %dx%d", w, h)
	}
	if w*120 != h*160 {
		t.Errorf("aspect ratio not preserved: %dx%d", w, h)
	}
	// A moderately-large source in a small box must not be downscaled below the
	// source (it stays at source resolution; the terminal downscales).
	w, h = upscaleTarget(800, 600, 10, 5)
	if w != 800 || h != 600 {
		t.Errorf("large source should stay at source size, got %dx%d", w, h)
	}
	// Clamp to maxUpscaleDim: a small source in a giant box, and a source that
	// itself exceeds the cap, both clamp the longest side.
	w, h = upscaleTarget(50, 50, 1000, 1000)
	if w > maxUpscaleDim || h > maxUpscaleDim {
		t.Errorf("target exceeds max: %dx%d", w, h)
	}
	w, h = upscaleTarget(1600, 1200, 10, 5)
	if w != maxUpscaleDim || h != 750 {
		t.Errorf("oversized source should clamp to %d, got %dx%d", maxUpscaleDim, w, h)
	}
}

func TestRenderKittyUpscalesToPNG(t *testing.T) {
	jp := smallJPEG(t, 16, 12) // tiny source -> should upscale
	out := renderKitty(jp, 60, 30)
	if out == "" {
		t.Fatal("renderKitty returned empty for a valid JPEG")
	}
	if !strings.Contains(out, "f=100") {
		t.Error("kitty payload should declare PNG (f=100)")
	}
	// Decode the transmitted PNG and confirm it is higher-resolution than the
	// 16x12 source (self-upscaled).
	png := decodeKittyPNG(t, out)
	if png.Bounds().Dx() <= 16 || png.Bounds().Dy() <= 12 {
		t.Errorf("transmitted image %v not upscaled beyond source", png.Bounds())
	}
}

func TestRenderITerm2UpscalesToPNG(t *testing.T) {
	jp := smallJPEG(t, 16, 12)
	out := RenderInline(ProtocolITerm2, jp, 60, 30)
	if !strings.HasPrefix(out, "\x1b]1337;File=") {
		t.Fatalf("unexpected prefix: %q", out[:min(len(out), 20)])
	}
	// Extract the base64 payload after the ':' and confirm it decodes as PNG
	// larger than the source.
	i := strings.IndexByte(out, ':')
	j := strings.IndexByte(out, '\a')
	if i < 0 || j < 0 {
		t.Fatal("malformed OSC 1337 payload")
	}
	raw, err := base64.StdEncoding.DecodeString(out[i+1 : j])
	if err != nil {
		t.Fatalf("payload not base64: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("payload not PNG: %v", err)
	}
	if img.Bounds().Dx() <= 16 {
		t.Errorf("iterm2 image %v not upscaled", img.Bounds())
	}
	// Invalid JPEG must fall back to raw bytes, not empty (never regress).
	if RenderInline(ProtocolITerm2, []byte{0xFF, 0xD8, 0xFF}, 12, 6) == "" {
		t.Error("iterm2 should fall back to raw bytes on decode failure")
	}
}

// decodeKittyPNG reassembles the base64 chunks from a Kitty APC sequence and
// decodes the PNG.
func decodeKittyPNG(t *testing.T, s string) image.Image {
	t.Helper()
	var enc strings.Builder
	for {
		semi := strings.IndexByte(s, ';')
		if semi < 0 {
			break
		}
		s = s[semi+1:]
		end := strings.Index(s, "\x1b\\")
		if end < 0 {
			break
		}
		enc.WriteString(s[:end])
		s = s[end+2:]
	}
	raw, err := base64.StdEncoding.DecodeString(enc.String())
	if err != nil {
		t.Fatalf("kitty payload not base64: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("kitty payload not PNG: %v", err)
	}
	return img
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
