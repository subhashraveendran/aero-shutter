package camera

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
)

func TestProfileForModel(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{"NIKON D5300", "Nikon D5300"},
		{"nikon d5300", "Nikon D5300"},
		{"NIKON D5200", "Nikon D5200"},
		{"NIKON D5500", "Nikon D5500"},
		{"NIKON D5600", "Nikon D5600"},
		{"NIKON D7100", "Nikon D7100"},
		{"NIKON D7200", "Nikon D7200"},
		{"NIKON D750", "Nikon D750"},
		{"NIKON D850", "Nikon D850"},
		{"NIKON D500", "Nikon D500"},
		{"NIKON Z 50", "Nikon Z50"},
		{"NIKON Z 6", "Nikon Z6"},
		{"NIKON Z 7", "Nikon Z7"},
		{"NIKON D90", "Generic PTP/IP camera"},
		{"", "Generic PTP/IP camera"},
	}
	for _, tc := range cases {
		if got := ProfileForModel(tc.model).Name; got != tc.want {
			t.Errorf("ProfileForModel(%q) = %q, want %q", tc.model, got, tc.want)
		}
	}
}

func TestProfileD500NotShadowedByD5xxx(t *testing.T) {
	// The D500 profile matches by the substring "D500", which must not
	// capture the four-digit D5xxx models listed before it.
	for _, model := range []string{"NIKON D5200", "NIKON D5300", "NIKON D5500", "NIKON D5600"} {
		if got := ProfileForModel(model).Name; got == "Nikon D500" {
			t.Errorf("ProfileForModel(%q) matched the D500 profile", model)
		}
	}
}

func TestKnownProfilesEnableLargeThumb(t *testing.T) {
	for _, p := range Profiles {
		if !p.LargeThumb {
			t.Errorf("profile %s does not enable LargeThumb", p.Name)
		}
	}
	if GenericProfile.LargeThumb {
		t.Error("GenericProfile must not enable the vendor LargeThumb operation")
	}
}

// fetchRecorder builds thumbFetchFuncs that log calls and return fixed
// results.
type fetchRecorder struct {
	calls []string
}

func (r *fetchRecorder) fn(name string, data []byte, err error) thumbFetchFunc {
	return func(context.Context, uint32) ([]byte, error) {
		r.calls = append(r.calls, name)
		return data, err
	}
}

func TestFetchThumbLargePreferred(t *testing.T) {
	var rec fetchRecorder
	var failed atomic.Bool
	data, err := fetchThumb(context.Background(), 1, true, &failed,
		rec.fn("large", []byte("big"), nil),
		rec.fn("std", []byte("small"), nil))
	if err != nil || string(data) != "big" {
		t.Fatalf("got (%q, %v), want the large thumb", data, err)
	}
	if !reflect.DeepEqual(rec.calls, []string{"large"}) {
		t.Errorf("calls = %v, want [large]", rec.calls)
	}
	if failed.Load() {
		t.Error("failed flag set after a successful large fetch")
	}
}

func TestFetchThumbFallsBackAndRemembers(t *testing.T) {
	var rec fetchRecorder
	var failed atomic.Bool
	notSupported := &ptpip.PTPError{Op: ptpip.OpNikonGetLargeThumb, Code: ptpip.RespOperationNotSupported}

	// First fetch: the camera rejects 0x90C4, we fall back to GetThumb.
	data, err := fetchThumb(context.Background(), 1, true, &failed,
		rec.fn("large", nil, notSupported),
		rec.fn("std", []byte("small"), nil))
	if err != nil || string(data) != "small" {
		t.Fatalf("got (%q, %v), want fallback thumb", data, err)
	}
	if !reflect.DeepEqual(rec.calls, []string{"large", "std"}) {
		t.Fatalf("calls = %v, want [large std]", rec.calls)
	}
	if !failed.Load() {
		t.Fatal("failed flag not set after PTP error")
	}

	// Second fetch: the vendor opcode must not be retried this session.
	rec.calls = nil
	data, err = fetchThumb(context.Background(), 2, true, &failed,
		rec.fn("large", nil, notSupported),
		rec.fn("std", []byte("small2"), nil))
	if err != nil || string(data) != "small2" {
		t.Fatalf("got (%q, %v), want fallback thumb", data, err)
	}
	if !reflect.DeepEqual(rec.calls, []string{"std"}) {
		t.Errorf("calls = %v, want [std] only", rec.calls)
	}
}

func TestFetchThumbTransportErrorPropagates(t *testing.T) {
	var rec fetchRecorder
	var failed atomic.Bool
	transport := errors.New("connection reset")
	_, err := fetchThumb(context.Background(), 1, true, &failed,
		rec.fn("large", nil, transport),
		rec.fn("std", []byte("small"), nil))
	if !errors.Is(err, transport) {
		t.Fatalf("err = %v, want the transport error", err)
	}
	if failed.Load() {
		t.Error("transport error must not disable the vendor opcode")
	}
	if !reflect.DeepEqual(rec.calls, []string{"large"}) {
		t.Errorf("calls = %v, want [large] only", rec.calls)
	}
}

func TestFetchThumbDisabled(t *testing.T) {
	var rec fetchRecorder
	var failed atomic.Bool
	data, err := fetchThumb(context.Background(), 1, false, &failed,
		rec.fn("large", []byte("big"), nil),
		rec.fn("std", []byte("small"), nil))
	if err != nil || string(data) != "small" {
		t.Fatalf("got (%q, %v), want standard thumb", data, err)
	}
	if !reflect.DeepEqual(rec.calls, []string{"std"}) {
		t.Errorf("calls = %v, want [std]", rec.calls)
	}
}

func TestMergeAddrs(t *testing.T) {
	got := mergeAddrs(
		[]string{"192.168.1.1", "", "192.168.1.1:15740", "10.0.0.2"},
		[]string{"192.168.1.1", "10.0.0.2:15740", "10.0.0.9"},
	)
	want := []string{"192.168.1.1", "10.0.0.2", "10.0.0.9"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mergeAddrs = %v, want %v", got, want)
	}
}

func TestCanonicalAddr(t *testing.T) {
	if CanonicalAddr("192.168.1.1") != "192.168.1.1:15740" {
		t.Error("bare host must gain the default port")
	}
	if CanonicalAddr("192.168.1.1:5000") != "192.168.1.1:5000" {
		t.Error("explicit port must be preserved")
	}
}
