package camera

import (
	"testing"

	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
)

func TestFormatFNumber(t *testing.T) {
	tests := []struct {
		raw  int64
		want string
	}{
		{560, "f/5.6"},
		{800, "f/8"},
		{120, "f/1.2"},
		{2200, "f/22"},
		{0, "0"},
	}
	for _, tt := range tests {
		if got := FormatFNumber(tt.raw); got != tt.want {
			t.Errorf("FormatFNumber(%d) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestFormatExposureTime(t *testing.T) {
	tests := []struct {
		raw  int64
		want string
	}{
		{40, "1/250s"},  // 4 ms
		{2, "1/5000s"},  // 0.2 ms
		{125, "1/80s"},  // 12.5 ms
		{10000, "1s"},   // exactly one second
		{20000, "2s"},   // two seconds
		{15000, "1.5s"}, // one and a half
		{0xFFFFFFFF, "Bulb"},
	}
	for _, tt := range tests {
		if got := FormatExposureTime(tt.raw); got != tt.want {
			t.Errorf("FormatExposureTime(%d) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestFormatExposureBias(t *testing.T) {
	tests := []struct {
		raw  int64
		want string
	}{
		{700, "+0.7 EV"},
		{-1300, "-1.3 EV"},
		{0, "+0.0 EV"},
		{2000, "+2.0 EV"},
	}
	for _, tt := range tests {
		if got := FormatExposureBias(tt.raw); got != tt.want {
			t.Errorf("FormatExposureBias(%d) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestFormatWhiteBalance(t *testing.T) {
	tests := []struct {
		raw  int64
		want string
	}{
		{2, "Auto"},
		{4, "Daylight"},
		{5, "Fluorescent"},
		{6, "Incandescent"},
		{7, "Flash"},
		{32784, "Cloudy"},
		{32785, "Shade"},
		{32786, "Kelvin"},
		{32787, "Custom"},
		{9999, "9999"},
	}
	for _, tt := range tests {
		if got := FormatWhiteBalance(tt.raw); got != tt.want {
			t.Errorf("FormatWhiteBalance(%d) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestFormatPropValueDispatch(t *testing.T) {
	if got := FormatPropValue(ptpip.PropFNumber, 560); got != "f/5.6" {
		t.Errorf("FNumber = %q", got)
	}
	if got := FormatPropValue(ptpip.PropBatteryLevel, 87); got != "87%" {
		t.Errorf("Battery = %q", got)
	}
	if got := FormatPropValue(ptpip.PropExposureIndex, 400); got != "400" {
		t.Errorf("ISO = %q", got)
	}
	if got := FormatPropValue(ptpip.PropExposureProgramMode, 1); got != "Manual" {
		t.Errorf("Mode = %q", got)
	}
	if got := FormatPropValue(ptpip.PropExposureProgramMode, 0x8050); got != "32848" {
		t.Errorf("vendor mode = %q", got)
	}
}

func TestRangeChoicesSmall(t *testing.T) {
	got := rangeChoices(100, 200, 10)
	if len(got) != 11 {
		t.Fatalf("length = %d, want 11", len(got))
	}
	if got[0] != 100 || got[10] != 200 || got[5] != 150 {
		t.Fatalf("choices = %v", got)
	}
}

func TestRangeChoicesCapped(t *testing.T) {
	got := rangeChoices(100, 25600, 1)
	if len(got) > maxRangeChoices {
		t.Fatalf("length = %d, want <= %d", len(got), maxRangeChoices)
	}
	if got[0] != 100 {
		t.Errorf("first = %d, want 100", got[0])
	}
	if got[len(got)-1] != 25600 {
		t.Errorf("last = %d, want 25600", got[len(got)-1])
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("choices not strictly increasing at %d: %v", i, got)
		}
	}
}

func TestRangeChoicesDegenerate(t *testing.T) {
	if got := rangeChoices(5, 5, 1); len(got) != 1 || got[0] != 5 {
		t.Fatalf("single-value range = %v", got)
	}
	if got := rangeChoices(10, 20, 0); got[0] != 10 || got[len(got)-1] != 20 {
		t.Fatalf("zero step range = %v", got)
	}
}

func TestSettingFromDescEnum(t *testing.T) {
	desc := ptpip.DevicePropDesc{
		Code:     ptpip.PropWhiteBalance,
		DataType: ptpip.DTUint16,
		Writable: true,
		Current:  ptpip.PropValue{Raw: 4},
		FormFlag: ptpip.FormEnum,
		Enum: []ptpip.PropValue{
			{Raw: 2}, {Raw: 4}, {Raw: 32784},
		},
	}
	s := settingFromDesc(desc)
	if s.Label != "White balance" || s.Formatted != "Daylight" || !s.Writable {
		t.Fatalf("setting = %+v", s)
	}
	if len(s.Choices) != 3 || s.Choices[2].Formatted != "Cloudy" {
		t.Fatalf("choices = %+v", s.Choices)
	}
}

func TestSettingFromDescRange(t *testing.T) {
	desc := ptpip.DevicePropDesc{
		Code:     ptpip.PropExposureIndex,
		DataType: ptpip.DTUint32,
		Writable: true,
		Current:  ptpip.PropValue{Raw: 400},
		FormFlag: ptpip.FormRange,
		Range:    &ptpip.PropRange{Min: 100, Max: 6400, Step: 100},
	}
	s := settingFromDesc(desc)
	if len(s.Choices) == 0 || len(s.Choices) > maxRangeChoices {
		t.Fatalf("choice count = %d", len(s.Choices))
	}
	if s.Choices[0].Raw != 100 || s.Choices[len(s.Choices)-1].Raw != 6400 {
		t.Fatalf("choices = %+v", s.Choices)
	}
	if s.Formatted != "400" {
		t.Fatalf("formatted = %q", s.Formatted)
	}
}
