package thumbnail

import (
	"bytes"
	"encoding/base64"
	"fmt"
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
	return ProtocolHalfBlock
}

// kittyChunkSize is the maximum base64 payload per Kitty APC chunk.
const kittyChunkSize = 4096

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
	png, err := jpegToPNG(jpeg)
	if err != nil {
		return ""
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
func renderITerm2(jpeg []byte, cols, rows int) string {
	enc := base64.StdEncoding.EncodeToString(jpeg)
	return fmt.Sprintf(
		"\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=1:%s\a",
		len(jpeg), cols, rows, enc,
	)
}

// Placeholder returns a bordered "No Preview" box of the given cell size for
// terminals without image support.
func Placeholder(cols, rows int) string {
	if cols < 4 || rows < 3 {
		return "[no preview]"
	}
	inner := cols - 2
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	label := "No Preview"
	for r := 0; r < rows-2; r++ {
		line := strings.Repeat(" ", inner)
		if r == (rows-2)/2 && inner >= len(label) {
			pad := (inner - len(label)) / 2
			line = strings.Repeat(" ", pad) + label + strings.Repeat(" ", inner-pad-len(label))
		}
		b.WriteString("│" + line + "│\n")
	}
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return b.String()
}
