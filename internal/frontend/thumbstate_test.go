package frontend

import (
	"strings"
	"testing"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
	"github.com/subhashraveendran/aero-shutter/internal/thumbnail"
)

func TestIsVideoFile(t *testing.T) {
	cases := []struct {
		f    camera.File
		want bool
	}{
		{camera.File{Format: ptpip.FormatMOV, Name: "DSC_0001.MOV"}, true},
		{camera.File{Format: ptpip.FormatJPEG, Name: "clip.mov"}, true},
		{camera.File{Format: ptpip.FormatJPEG, Name: "clip.MP4"}, true},
		{camera.File{Format: ptpip.FormatJPEG, Name: "clip.avi"}, true},
		{camera.File{Format: ptpip.FormatJPEG, Name: "DSC_0001.JPG"}, false},
		{camera.File{Format: ptpip.FormatNEF, Name: "DSC_0001.NEF"}, false},
	}
	for _, tc := range cases {
		if got := isVideoFile(tc.f); got != tc.want {
			t.Errorf("isVideoFile(%q,%v) = %v, want %v", tc.f.Name, tc.f.Format, got, tc.want)
		}
	}
}

func TestRenderThumbStates(t *testing.T) {
	const h = uint32(0x42)
	const cols, rows = 24, 6

	base := func() Model {
		return Model{
			proto: thumbnail.ProtocolHalfBlock,
			files: []camera.File{{Handle: h, Name: "DSC_0001.MOV", Format: ptpip.FormatMOV}},
		}
	}

	// Loading state.
	m := base()
	m.thumbLoading = true
	m.thumbLoadingHandle = h
	if out := m.renderThumb(h, cols, rows); !strings.Contains(out, "Loading preview") {
		t.Errorf("loading state = %q", firstLine(out))
	}

	// Error state.
	m = base()
	m.thumbErr = true
	m.thumbErrHandle = h
	if out := m.renderThumb(h, cols, rows); !strings.Contains(out, "Preview unavailable") {
		t.Errorf("error state = %q", firstLine(out))
	}

	// Empty + video.
	m = base()
	m.thumbHandle = h
	m.thumbData = nil
	if out := m.renderThumb(h, cols, rows); !strings.Contains(out, "Video — no preview") {
		t.Errorf("video empty state = %q", firstLine(out))
	}

	// Empty + non-video.
	m = base()
	m.files[0] = camera.File{Handle: h, Name: "DSC_0001.JPG", Format: ptpip.FormatJPEG}
	m.thumbHandle = h
	m.thumbData = nil
	if out := m.renderThumb(h, cols, rows); !strings.Contains(out, "No preview available") {
		t.Errorf("non-video empty state = %q", firstLine(out))
	}

	// Default placeholder (nothing known about this handle).
	m = base()
	if out := m.renderThumb(h, cols, rows); !strings.Contains(out, "No Preview") {
		t.Errorf("default state = %q", firstLine(out))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
