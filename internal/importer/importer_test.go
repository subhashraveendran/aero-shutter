package importer

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
	"github.com/subhashraveendran/aero-shutter/internal/database"
	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
)

func TestDestPath(t *testing.T) {
	f := camera.File{
		Name:        "DSC_0042.NEF",
		CaptureTime: time.Date(2024, 3, 5, 9, 30, 0, 0, time.Local),
	}
	got := DestPath("/photos", f)
	want := filepath.Join("/photos", "2024", "03-05", "DSC_0042.NEF")
	if got != want {
		t.Errorf("DestPath = %q, want %q", got, want)
	}
}

func TestDestPathUnknownDate(t *testing.T) {
	f := camera.File{Name: "DSC_0001.JPG"}
	got := DestPath("/photos", f)
	want := filepath.Join("/photos", "unknown-date", "DSC_0001.JPG")
	if got != want {
		t.Errorf("DestPath = %q, want %q", got, want)
	}
}

func TestMatches(t *testing.T) {
	nef := camera.File{Handle: 1, Name: "a.nef", Size: 10, Format: ptpip.FormatNEF}
	jpg := camera.File{Handle: 2, Name: "b.jpg", Size: 20, Format: ptpip.FormatJPEG}
	mov := camera.File{Handle: 3, Name: "c.mov", Size: 30, Format: ptpip.FormatMOV}
	imported := map[string]bool{database.Key(2, "b.jpg", 20): true}

	cases := []struct {
		name string
		f    camera.File
		opts Options
		want bool
	}{
		{"all matches nef", nef, Options{Filter: FilterAll}, true},
		{"all matches mov", mov, Options{Filter: FilterAll}, true},
		{"new skips imported", jpg, Options{Filter: FilterNew}, false},
		{"new keeps fresh", nef, Options{Filter: FilterNew}, true},
		{"raw keeps nef", nef, Options{Filter: FilterRAW}, true},
		{"raw skips jpg", jpg, Options{Filter: FilterRAW}, false},
		{"jpeg keeps jpg", jpg, Options{Filter: FilterJPEG}, true},
		{"jpeg skips mov", mov, Options{Filter: FilterJPEG}, false},
		{"selected keeps chosen", nef, Options{Filter: FilterSelected, Selected: map[uint32]bool{1: true}}, true},
		{"selected skips others", jpg, Options{Filter: FilterSelected, Selected: map[uint32]bool{1: true}}, false},
	}
	for _, tc := range cases {
		if got := Matches(tc.f, tc.opts, imported); got != tc.want {
			t.Errorf("%s: Matches = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSpeedometer(t *testing.T) {
	s := newSpeedometer(time.Minute)
	s.add(0)
	time.Sleep(20 * time.Millisecond)
	s.add(1 << 20)
	rate := s.rate()
	if rate <= 0 {
		t.Fatalf("rate = %f, want > 0", rate)
	}
}
