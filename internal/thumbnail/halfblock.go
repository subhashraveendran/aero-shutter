package thumbnail

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
)

// The half-block renderer is the universal preview fallback: it draws the
// thumbnail with '▀' (upper half block) characters, using the foreground
// color for the top pixel and the background color for the bottom pixel of
// each cell, so every terminal cell carries a 1x2 pixel pair. It works in any
// terminal that supports ANSI colors and needs no image protocol.

// halfBlock is the glyph whose foreground paints the top pixel and whose
// background paints the bottom pixel of a cell.
const halfBlock = "▀"

// detectTruecolor reports whether the terminal advertises 24-bit color
// support via $COLORTERM ("truecolor" or "24bit").
func detectTruecolor(getenv func(string) string) bool {
	ct := strings.ToLower(getenv("COLORTERM"))
	return strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit")
}

// rgb is an 8-bit-per-channel pixel used by the scaler.
type rgb struct{ r, g, b uint8 }

// ansi256 maps an RGB color onto the xterm 256-color 6x6x6 cube
// (indices 16-231) for terminals without 24-bit color support.
func ansi256(r, g, b uint8) int {
	q := func(v uint8) int {
		i := int(v) * 6 / 256
		if i > 5 {
			i = 5
		}
		return i
	}
	return 16 + 36*q(r) + 6*q(g) + q(b)
}

// boxScale downsamples img to w x h pixels by averaging each source box
// (falling back to nearest-neighbor when upscaling). No external deps.
func boxScale(img image.Image, w, h int) []rgb {
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	out := make([]rgb, w*h)
	for y := 0; y < h; y++ {
		y0 := bounds.Min.Y + y*srcH/h
		y1 := bounds.Min.Y + (y+1)*srcH/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0 := bounds.Min.X + x*srcW/w
			x1 := bounds.Min.X + (x+1)*srcW/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var rs, gs, bs, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, b, _ := img.At(sx, sy).RGBA()
					rs += uint64(r >> 8)
					gs += uint64(g >> 8)
					bs += uint64(b >> 8)
					n++
				}
			}
			out[y*w+x] = rgb{uint8(rs / n), uint8(gs / n), uint8(bs / n)}
		}
	}
	return out
}

// fitBox returns the largest pixel size that fits inside cols x rows*2 while
// preserving the image aspect ratio (letterboxing instead of cropping). The
// height is forced even so pixels pair up into half-block cells.
func fitBox(srcW, srcH, cols, rows int) (w, h int) {
	maxW, maxH := cols, rows*2
	if srcW <= 0 || srcH <= 0 || maxW < 1 || maxH < 2 {
		return 0, 0
	}
	w, h = maxW, srcH*maxW/srcW
	if h > maxH {
		w, h = srcW*maxH/srcH, maxH
	}
	if w < 1 {
		w = 1
	}
	if h < 2 {
		h = 2
	}
	h -= h % 2
	return w, h
}

// renderHalfBlock decodes a JPEG thumbnail and renders it as ANSI half-block
// cells fitting inside cols x rows terminal cells. It returns "" if the image
// cannot be decoded, letting the caller fall back to the placeholder.
func renderHalfBlock(data []byte, cols, rows int, truecolor bool) string {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return renderHalfBlockImage(img, cols, rows, truecolor)
}

// renderHalfBlockImage scales img to fit cols x rows cells and emits one
// independent line per cell row. Every line ends with an SGR reset so the
// output cannot bleed colors into the surrounding Bubble Tea layout.
func renderHalfBlockImage(img image.Image, cols, rows int, truecolor bool) string {
	w, h := fitBox(img.Bounds().Dx(), img.Bounds().Dy(), cols, rows)
	if w == 0 || h == 0 {
		return ""
	}
	px := boxScale(img, w, h)
	indent := strings.Repeat(" ", (cols-w)/2) // center inside the cell box

	var b strings.Builder
	for y := 0; y < h; y += 2 {
		if y > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(indent)
		for x := 0; x < w; x++ {
			top := px[y*w+x]
			bot := px[(y+1)*w+x]
			if truecolor {
				fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%s",
					top.r, top.g, top.b, bot.r, bot.g, bot.b, halfBlock)
			} else {
				fmt.Fprintf(&b, "\x1b[38;5;%dm\x1b[48;5;%dm%s",
					ansi256(top.r, top.g, top.b), ansi256(bot.r, bot.g, bot.b), halfBlock)
			}
		}
		b.WriteString("\x1b[0m")
	}
	return b.String()
}
