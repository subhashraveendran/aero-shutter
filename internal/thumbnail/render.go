package thumbnail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
)

// jpegToPNG transcodes a JPEG image to PNG, as the Kitty graphics protocol
// only accepts PNG (f=100) or raw pixel data.
func jpegToPNG(data []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Protocol identifies a terminal inline-image protocol.
type Protocol int

// Supported inline-image protocols.
const (
	// ProtocolNone means no rendering at all; callers should render a
	// placeholder instead.
	ProtocolNone Protocol = iota
	// ProtocolKitty is the Kitty graphics protocol (Kitty, WezTerm, Ghostty).
	ProtocolKitty
	// ProtocolITerm2 is the iTerm2 OSC 1337 inline image protocol.
	ProtocolITerm2
	// ProtocolHalfBlock draws the image with ANSI-colored '▀' half-block
	// cells. It works in any color terminal and is the universal fallback.
	ProtocolHalfBlock
)

// String returns the protocol name.
func (p Protocol) String() string {
	switch p {
	case ProtocolKitty:
		return "kitty"
	case ProtocolITerm2:
		return "iterm2"
	case ProtocolHalfBlock:
		return "halfblock"
	default:
		return "none"
	}
}

// Inline reports whether the protocol draws via terminal escape sequences
// that occupy no layout lines of their own (Kitty, iTerm2), as opposed to
// the half-block renderer whose output is ordinary text lines.
func (p Protocol) Inline() bool {
	return p == ProtocolKitty || p == ProtocolITerm2
}

// DetectProtocol inspects the environment ($TERM, $TERM_PROGRAM,
// $KITTY_WINDOW_ID, ...) and returns the best supported protocol,
// preferring Kitty, then iTerm2, then the always-available half-block
// renderer.
func DetectProtocol() Protocol {
	return detectProtocol(os.Getenv)
}

// detectProtocol is the testable core of DetectProtocol.
func detectProtocol(getenv func(string) string) Protocol {
	term := strings.ToLower(getenv("TERM"))
	prog := strings.ToLower(getenv("TERM_PROGRAM"))

	if getenv("KITTY_WINDOW_ID") != "" || strings.Contains(term, "kitty") {
		return ProtocolKitty
	}
	if getenv("WEZTERM_EXECUTABLE") != "" || prog == "wezterm" {
		return ProtocolKitty
	}
	if getenv("GHOSTTY_RESOURCES_DIR") != "" || prog == "ghostty" || strings.Contains(term, "ghostty") {
		return ProtocolKitty
	}
	if prog == "iterm.app" || getenv("ITERM_SESSION_ID") != "" {
		return ProtocolITerm2
	}
	if prog == "mintty" {
		return ProtocolITerm2
	}
	// VS Code's integrated terminal supports the iTerm2 OSC 1337 protocol
	// when the "terminal.integrated.enableImages" setting is on. The setting
	// cannot be detected from the environment; users who left it off can
	// force the half-block renderer via the previewMode config field.
	if prog == "vscode" {
		return ProtocolITerm2
	}
	return ProtocolHalfBlock
}

// ProtocolFromMode resolves the previewMode configuration value: "kitty",
// "iterm2" and "halfblock" force that protocol, while "auto" (or any other
// value) falls back to environment detection via DetectProtocol.
func ProtocolFromMode(mode string) Protocol {
	return protocolFromMode(mode, os.Getenv)
}

// protocolFromMode is the testable core of ProtocolFromMode.
func protocolFromMode(mode string, getenv func(string) string) Protocol {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "kitty":
		return ProtocolKitty
	case "iterm2":
		return ProtocolITerm2
	case "halfblock":
		return ProtocolHalfBlock
	default:
		return detectProtocol(getenv)
	}
}

// kittyChunkSize is the maximum base64 payload per Kitty APC chunk.
const kittyChunkSize = 4096

// Cell pixel dimensions used to convert a terminal cell box into a target
// pixel size for self-upscaling. These are conservative averages for common
// terminal fonts (a cell is roughly 8px wide and 16px tall).
const (
	cellPxW = 8
	cellPxH = 16
	// maxUpscaleDim caps the longest side of the upscaled image so we never
	// build an absurdly large PNG for a huge preview box.
	maxUpscaleDim = 1000
)

// upscaleTarget computes the target pixel size for an image displayed in a
// cols x rows cell box. It preserves the source aspect ratio, never downscales
// below the source (only upscales when the source is smaller than the box),
// and clamps the longest side to maxUpscaleDim.
func upscaleTarget(srcW, srcH, cols, rows int) (w, h int) {
	if srcW <= 0 || srcH <= 0 {
		return srcW, srcH
	}
	// Pixel box the terminal will paint the image into.
	boxW := cols * cellPxW
	boxH := rows * cellPxH
	if boxW < 1 {
		boxW = 1
	}
	if boxH < 1 {
		boxH = 1
	}
	// Largest size fitting the box while preserving aspect ratio.
	w, h = boxW, srcH*boxW/srcW
	if h > boxH {
		w, h = srcW*boxH/srcH, boxH
	}
	// Never downscale: if the fitted size is smaller than the source in either
	// dimension, keep the source resolution (the terminal will downscale).
	if w <= srcW || h <= srcH {
		w, h = srcW, srcH
	}
	// Clamp the longest side to the max, preserving aspect ratio.
	if w > maxUpscaleDim || h > maxUpscaleDim {
		if w >= h {
			h = h * maxUpscaleDim / w
			w = maxUpscaleDim
		} else {
			w = w * maxUpscaleDim / h
			h = maxUpscaleDim
		}
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// upscaledPNG decodes the JPEG thumbnail, bilinearly upscales it to a target
// pixel size derived from the cols x rows cell box (only when the source is
// smaller than the display box), and re-encodes it as PNG. Handing the
// terminal a higher-resolution, smoothly interpolated source makes its own
// scaling look crisp instead of blocky. It returns an error on any
// decode/encode failure so callers can fall back to the raw thumbnail.
func upscaledPNG(data []byte, cols, rows int) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	srcW, srcH := img.Bounds().Dx(), img.Bounds().Dy()
	tw, th := upscaleTarget(srcW, srcH, cols, rows)
	if tw != srcW || th != srcH {
		img = scaleToImage(img, tw, th)
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// scaleToImage resizes img to w x h pixels using bilinear interpolation and
// returns it as an *image.RGBA so it can be PNG-encoded. It shares the same
// interpolation math as bilinearScale.
func scaleToImage(img image.Image, w, h int) *image.RGBA {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	px := bilinearScale(img, w, h)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := px[y*w+x]
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = p.r
			dst.Pix[i+1] = p.g
			dst.Pix[i+2] = p.b
			dst.Pix[i+3] = 0xFF
		}
	}
	return dst
}

// RenderInline returns the escape sequence that draws the JPEG at the cursor
// position sized to the given cell box, using the specified protocol. For
// ProtocolHalfBlock the result is plain text lines (one per cell row, each
// ending with an SGR reset). It returns "" for ProtocolNone, empty data or a
// thumbnail that cannot be decoded, letting the caller show a placeholder.
func RenderInline(proto Protocol, jpeg []byte, cols, rows int) string {
	if len(jpeg) == 0 {
		return ""
	}
	switch proto {
	case ProtocolKitty:
		return renderKitty(jpeg, cols, rows)
	case ProtocolITerm2:
		return renderITerm2(jpeg, cols, rows)
	case ProtocolHalfBlock:
		return renderHalfBlock(jpeg, cols, rows, detectTruecolor(os.Getenv))
	default:
		return ""
	}
}

// renderKitty emits the image as base64-chunked APC escape sequences using
// the Kitty graphics protocol (a=T transmit-and-display, f=100 PNG). The
// camera hands us JPEG thumbnails, so they are transcoded to PNG first;
// on a decode failure an empty string is returned and the caller falls back
// to the placeholder.
func renderKitty(jpeg []byte, cols, rows int) string {
	// Prefer a self-upscaled PNG so the terminal has a high-resolution,
	// bilinear-smoothed source to scale. Fall back to a plain JPEG->PNG
	// transcode, and only return "" if even that fails to decode.
	png, err := upscaledPNG(jpeg, cols, rows)
	if err != nil {
		png, err = jpegToPNG(jpeg)
		if err != nil {
			return ""
		}
	}
	enc := base64.StdEncoding.EncodeToString(png)
	var b strings.Builder
	first := true
	for len(enc) > 0 {
		chunk := enc
		if len(chunk) > kittyChunkSize {
			chunk = chunk[:kittyChunkSize]
		}
		enc = enc[len(chunk):]
		more := 0
		if len(enc) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(&b, "\x1b_Ga=T,f=100,c=%d,r=%d,m=%d;%s\x1b\\", cols, rows, more, chunk)
			first = false
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
	}
	return b.String()
}

// renderITerm2 emits an OSC 1337 File= inline image sized in character cells.
// iTerm2 accepts PNG bytes as well as JPEG, so we transmit a self-upscaled PNG
// (higher-resolution, bilinear-smoothed) for crisper scaling, falling back to
// the raw JPEG bytes on any decode/encode failure so we never regress.
func renderITerm2(jpeg []byte, cols, rows int) string {
	payload := jpeg
	if png, err := upscaledPNG(jpeg, cols, rows); err == nil {
		payload = png
	}
	enc := base64.StdEncoding.EncodeToString(payload)
	return fmt.Sprintf(
		"\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=1:%s\a",
		len(payload), cols, rows, enc,
	)
}

// LabeledBox returns a rounded bordered box of the given cell size with the
// given label centered vertically and horizontally. It degrades to plain text
// ("[" + label + "]") when the box is too small to draw. The label width is
// measured in runes so multi-byte labels center correctly.
func LabeledBox(label string, cols, rows int) string {
	labelW := len([]rune(label))
	if cols < 4 || rows < 3 {
		return "[" + label + "]"
	}
	inner := cols - 2
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	for r := 0; r < rows-2; r++ {
		line := strings.Repeat(" ", inner)
		if r == (rows-2)/2 && inner >= labelW {
			pad := (inner - labelW) / 2
			line = strings.Repeat(" ", pad) + label + strings.Repeat(" ", inner-pad-labelW)
		}
		b.WriteString("│" + line + "│\n")
	}
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return b.String()
}

// Placeholder returns a bordered "No Preview" box of the given cell size for
// terminals without image support. When the box is too small to draw it
// degrades to the plain "[no preview]" text (preserving the long-standing
// contract callers rely on).
func Placeholder(cols, rows int) string {
	if cols < 4 || rows < 3 {
		return "[no preview]"
	}
	return LabeledBox("No Preview", cols, rows)
}
