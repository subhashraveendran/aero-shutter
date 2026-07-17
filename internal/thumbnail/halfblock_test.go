package thumbnail

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
)

func TestDetectTruecolor(t *testing.T) {
	cases := []struct {
		colorterm string
		want      bool
	}{
		{"truecolor", true},
		{"24bit", true},
		{"TRUECOLOR", true},
		{"256color", false},
		{"", false},
	}
	for _, tc := range cases {
		getenv := func(k string) string {
			if k == "COLORTERM" {
				return tc.colorterm
			}
			return ""
		}
		if got := detectTruecolor(getenv); got != tc.want {
			t.Errorf("detectTruecolor(COLORTERM=%q) = %v, want %v", tc.colorterm, got, tc.want)
		}
	}
}

func TestAnsi256(t *testing.T) {
	cases := []struct {
		r, g, b uint8
		want    int
	}{
		{0, 0, 0, 16},        // cube origin
		{255, 255, 255, 231}, // cube max
		{255, 0, 0, 16 + 36*5},
		{0, 255, 0, 16 + 6*5},
		{0, 0, 255, 16 + 5},
		{128, 128, 128, 16 + 36*3 + 6*3 + 3},
	}
	for _, tc := range cases {
		if got := ansi256(tc.r, tc.g, tc.b); got != tc.want {
			t.Errorf("ansi256(%d,%d,%d) = %d, want %d", tc.r, tc.g, tc.b, got, tc.want)
		}
	}
}

// testImage builds a 2x4 image: left column red over green over blue over
// white, right column all black.
func testImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 2, 4))
	left := []color.RGBA{
		{255, 0, 0, 255},
		{0, 255, 0, 255},
		{0, 0, 255, 255},
		{255, 255, 255, 255},
	}
	for y := 0; y < 4; y++ {
		img.Set(0, y, left[y])
		img.Set(1, y, color.RGBA{0, 0, 0, 255})
	}
	return img
}

func TestBilinearScaleInterpolatesMidpoints(t *testing.T) {
	// 2x2 checkerboard: black/white over white/black. Upscaling to 3x3 puts
	// the destination center pixel exactly between all four sources, so it
	// must interpolate to mid-gray, while the corners map straight onto the
	// source pixels.
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{0, 0, 0, 255})
	img.Set(1, 0, color.RGBA{255, 255, 255, 255})
	img.Set(0, 1, color.RGBA{255, 255, 255, 255})
	img.Set(1, 1, color.RGBA{0, 0, 0, 255})

	px := bilinearScale(img, 3, 3)
	if len(px) != 9 {
		t.Fatalf("got %d pixels, want 9", len(px))
	}
	if px[0] != (rgb{0, 0, 0}) {
		t.Errorf("top-left = %v, want black", px[0])
	}
	if px[2] != (rgb{255, 255, 255}) {
		t.Errorf("top-right = %v, want white", px[2])
	}
	center := px[4]
	for _, ch := range []uint8{center.r, center.g, center.b} {
		if ch < 126 || ch > 129 {
			t.Errorf("center channel = %d, want ~127 (bilinear midpoint)", ch)
		}
	}
	// Edge midpoints sit halfway between two sources horizontally: the top
	// middle pixel blends black and white to mid-gray too.
	top := px[1]
	if top.r < 126 || top.r > 129 {
		t.Errorf("top-middle = %v, want ~127 gray", top)
	}
}

func TestScalePixelsChoosesFilter(t *testing.T) {
	// A 2x1 black|white image upscaled to 4x1 must show interpolated
	// intermediate values (bilinear), not duplicated blocks (nearest).
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{0, 0, 0, 255})
	img.Set(1, 0, color.RGBA{255, 255, 255, 255})
	px := scalePixels(img, 4, 1)
	if px[1].r == px[0].r || px[1].r == px[3].r {
		t.Errorf("upscale not interpolated: %v", px)
	}
	if px[1].r >= px[2].r {
		t.Errorf("gradient not monotonic: %v", px)
	}
	// Downscaling still box-averages: a 2x2 black/white checker reduced to
	// 1x1 averages to mid-gray.
	checker := image.NewRGBA(image.Rect(0, 0, 2, 2))
	checker.Set(0, 0, color.RGBA{0, 0, 0, 255})
	checker.Set(1, 0, color.RGBA{255, 255, 255, 255})
	checker.Set(0, 1, color.RGBA{255, 255, 255, 255})
	checker.Set(1, 1, color.RGBA{0, 0, 0, 255})
	avg := scalePixels(checker, 1, 1)
	if avg[0].r < 126 || avg[0].r > 129 {
		t.Errorf("downscale average = %v, want ~127 gray", avg[0])
	}
}

func TestRenderHalfBlockImageTruecolor(t *testing.T) {
	out := renderHalfBlockImage(testImage(), 2, 2, true)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (one per cell row)", len(lines))
	}
	for i, line := range lines {
		if !strings.HasSuffix(line, "\x1b[0m") {
			t.Errorf("line %d does not end with SGR reset: %q", i, line)
		}
		if strings.Count(line, halfBlock) != 2 {
			t.Errorf("line %d has %d half-blocks, want 2", i, strings.Count(line, halfBlock))
		}
	}
	// First cell: red top pixel over green bottom pixel.
	if !strings.Contains(lines[0], "\x1b[38;2;255;0;0m\x1b[48;2;0;255;0m") {
		t.Errorf("line 0 missing red-over-green cell: %q", lines[0])
	}
	// Second row, first cell: blue over white.
	if !strings.Contains(lines[1], "\x1b[38;2;0;0;255m\x1b[48;2;255;255;255m") {
		t.Errorf("line 1 missing blue-over-white cell: %q", lines[1])
	}
}

func TestRenderHalfBlockImage256(t *testing.T) {
	out := renderHalfBlockImage(testImage(), 2, 2, false)
	if strings.Contains(out, "38;2;") || strings.Contains(out, "48;2;") {
		t.Error("256-color output must not contain 24-bit sequences")
	}
	// Red (196) over green (46) in the first cell.
	if !strings.Contains(out, "\x1b[38;5;196m\x1b[48;5;46m") {
		t.Errorf("missing quantized red-over-green cell: %q", out)
	}
}

func TestRenderHalfBlockAspectAndCentering(t *testing.T) {
	// A 4x4 source in an 8x2 cell box (8x4 pixels) scales to 4x4 pixels
	// and is centered with 2 leading spaces per line.
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	out := renderHalfBlockImage(img, 8, 2, true)
	for i, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "  \x1b[") {
			t.Errorf("line %d not centered: %q", i, line)
		}
		if strings.Count(line, halfBlock) != 4 {
			t.Errorf("line %d has %d cells, want 4", i, strings.Count(line, halfBlock))
		}
	}
}

func TestRenderHalfBlockJPEG(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), nil); err != nil {
		t.Fatal(err)
	}
	out := renderHalfBlock(buf.Bytes(), 2, 2, true)
	if out == "" {
		t.Fatal("valid JPEG rendered as empty string")
	}
	if len(strings.Split(out, "\n")) != 2 {
		t.Error("JPEG render has wrong line count")
	}
	if renderHalfBlock([]byte("not a jpeg"), 2, 2, true) != "" {
		t.Error("invalid JPEG must render as empty string")
	}
}

func TestRenderInlineHalfBlock(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), nil); err != nil {
		t.Fatal(err)
	}
	out := RenderInline(ProtocolHalfBlock, buf.Bytes(), 4, 4)
	if out == "" {
		t.Fatal("half-block RenderInline returned empty string for valid JPEG")
	}
	if !strings.Contains(out, halfBlock) {
		t.Error("half-block output missing half-block glyphs")
	}
	if RenderInline(ProtocolHalfBlock, []byte("bad"), 4, 4) != "" {
		t.Error("undecodable data must fall through to placeholder")
	}
}

func TestFitBox(t *testing.T) {
	cases := []struct {
		srcW, srcH, cols, rows int
		w, h                   int
	}{
		{160, 120, 40, 10, 26, 20}, // height-limited, forced even
		{160, 120, 20, 30, 20, 14}, // width-limited
		{0, 0, 20, 10, 0, 0},       // degenerate source
		{160, 120, 0, 0, 0, 0},     // degenerate box
	}
	for _, tc := range cases {
		w, h := fitBox(tc.srcW, tc.srcH, tc.cols, tc.rows)
		if w != tc.w || h != tc.h {
			t.Errorf("fitBox(%d,%d,%d,%d) = %d,%d want %d,%d",
				tc.srcW, tc.srcH, tc.cols, tc.rows, w, h, tc.w, tc.h)
		}
	}
}
