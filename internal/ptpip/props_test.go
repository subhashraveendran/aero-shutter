package ptpip

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDecodeDevicePropDescEnumUint16(t *testing.T) {
	var b []byte
	b = binary.LittleEndian.AppendUint16(b, uint16(PropWhiteBalance))
	b = binary.LittleEndian.AppendUint16(b, uint16(DTUint16))
	b = append(b, 1)                               // GetSet: writable
	b = binary.LittleEndian.AppendUint16(b, 2)     // factory default: Auto
	b = binary.LittleEndian.AppendUint16(b, 4)     // current: Daylight
	b = append(b, FormEnum)                        // enum form
	b = binary.LittleEndian.AppendUint16(b, 3)     // 3 values
	b = binary.LittleEndian.AppendUint16(b, 2)     // Auto
	b = binary.LittleEndian.AppendUint16(b, 4)     // Daylight
	b = binary.LittleEndian.AppendUint16(b, 32784) // Cloudy

	pd, err := DecodeDevicePropDesc(b)
	if err != nil {
		t.Fatalf("DecodeDevicePropDesc: %v", err)
	}
	if pd.Code != PropWhiteBalance || pd.DataType != DTUint16 {
		t.Fatalf("code/type = 0x%04X/0x%04X", uint16(pd.Code), uint16(pd.DataType))
	}
	if !pd.Writable {
		t.Fatal("expected writable")
	}
	if pd.Factory.Raw != 2 || pd.Current.Raw != 4 {
		t.Fatalf("factory/current = %d/%d, want 2/4", pd.Factory.Raw, pd.Current.Raw)
	}
	if pd.FormFlag != FormEnum || pd.Range != nil {
		t.Fatalf("form = %d, range = %v", pd.FormFlag, pd.Range)
	}
	want := []int64{2, 4, 32784}
	if len(pd.Enum) != len(want) {
		t.Fatalf("enum length = %d, want %d", len(pd.Enum), len(want))
	}
	for i, w := range want {
		if pd.Enum[i].Raw != w {
			t.Errorf("enum[%d] = %d, want %d", i, pd.Enum[i].Raw, w)
		}
	}
}

func TestDecodeDevicePropDescEnumInt16(t *testing.T) {
	i16 := func(v int16) uint16 { return uint16(v) }
	var b []byte
	b = binary.LittleEndian.AppendUint16(b, uint16(PropExposureBias))
	b = binary.LittleEndian.AppendUint16(b, uint16(DTInt16))
	b = append(b, 1)
	b = binary.LittleEndian.AppendUint16(b, 0)         // factory: 0
	b = binary.LittleEndian.AppendUint16(b, i16(-700)) // current: -0.7 EV
	b = append(b, FormEnum)
	b = binary.LittleEndian.AppendUint16(b, 3)
	b = binary.LittleEndian.AppendUint16(b, i16(-1000))
	b = binary.LittleEndian.AppendUint16(b, 0)
	b = binary.LittleEndian.AppendUint16(b, 1000)

	pd, err := DecodeDevicePropDesc(b)
	if err != nil {
		t.Fatalf("DecodeDevicePropDesc: %v", err)
	}
	if pd.Current.Raw != -700 {
		t.Fatalf("current = %d, want -700", pd.Current.Raw)
	}
	if pd.Enum[0].Raw != -1000 || pd.Enum[1].Raw != 0 || pd.Enum[2].Raw != 1000 {
		t.Fatalf("enum = %v", pd.Enum)
	}
}

func TestDecodeDevicePropDescRangeUint32(t *testing.T) {
	var b []byte
	b = binary.LittleEndian.AppendUint16(b, uint16(PropExposureIndex))
	b = binary.LittleEndian.AppendUint16(b, uint16(DTUint32))
	b = append(b, 1)
	b = binary.LittleEndian.AppendUint32(b, 100) // factory
	b = binary.LittleEndian.AppendUint32(b, 400) // current
	b = append(b, FormRange)
	b = binary.LittleEndian.AppendUint32(b, 100)   // min
	b = binary.LittleEndian.AppendUint32(b, 12800) // max
	b = binary.LittleEndian.AppendUint32(b, 100)   // step

	pd, err := DecodeDevicePropDesc(b)
	if err != nil {
		t.Fatalf("DecodeDevicePropDesc: %v", err)
	}
	if pd.Current.Raw != 400 {
		t.Fatalf("current = %d, want 400", pd.Current.Raw)
	}
	if pd.FormFlag != FormRange || pd.Range == nil {
		t.Fatalf("form = %d, range = %v", pd.FormFlag, pd.Range)
	}
	if pd.Range.Min != 100 || pd.Range.Max != 12800 || pd.Range.Step != 100 {
		t.Fatalf("range = %+v", *pd.Range)
	}
	if pd.Enum != nil {
		t.Fatalf("unexpected enum: %v", pd.Enum)
	}
}

func TestDecodeDevicePropDescString(t *testing.T) {
	var b []byte
	b = binary.LittleEndian.AppendUint16(b, 0xD000)
	b = binary.LittleEndian.AppendUint16(b, uint16(DTStr))
	b = append(b, 0) // read-only
	b = append(b, EncodePTPString("abc")...)
	b = append(b, EncodePTPString("def")...)
	b = append(b, FormNone)

	pd, err := DecodeDevicePropDesc(b)
	if err != nil {
		t.Fatalf("DecodeDevicePropDesc: %v", err)
	}
	if pd.Writable {
		t.Fatal("expected read-only")
	}
	if pd.Factory.Str != "abc" || pd.Current.Str != "def" {
		t.Fatalf("factory/current = %q/%q", pd.Factory.Str, pd.Current.Str)
	}
}

func TestDecodeDevicePropDescTruncated(t *testing.T) {
	var b []byte
	b = binary.LittleEndian.AppendUint16(b, uint16(PropExposureIndex))
	b = binary.LittleEndian.AppendUint16(b, uint16(DTUint32))
	b = append(b, 1)
	b = binary.LittleEndian.AppendUint32(b, 100)
	// current value and the rest missing
	if _, err := DecodeDevicePropDesc(b); err == nil {
		t.Fatal("expected an error for a truncated dataset")
	}
}

func TestEncodePropValue(t *testing.T) {
	tests := []struct {
		name string
		dt   DataType
		v    PropValue
		want []byte
	}{
		{"uint8", DTUint8, PropValue{Raw: 0x2A}, []byte{0x2A}},
		{"int16 negative", DTInt16, PropValue{Raw: -700}, []byte{0x44, 0xFD}},
		{"uint16", DTUint16, PropValue{Raw: 32784}, []byte{0x10, 0x80}},
		{"uint32", DTUint32, PropValue{Raw: 12800}, []byte{0x00, 0x32, 0x00, 0x00}},
		{"uint64", DTUint64, PropValue{Raw: 1}, []byte{1, 0, 0, 0, 0, 0, 0, 0}},
		{"str", DTStr, PropValue{Str: "ab"}, []byte{3, 'a', 0, 'b', 0, 0, 0}},
	}
	for _, tt := range tests {
		got, err := EncodePropValue(tt.dt, tt.v)
		if err != nil {
			t.Errorf("%s: EncodePropValue: %v", tt.name, err)
			continue
		}
		if !bytes.Equal(got, tt.want) {
			t.Errorf("%s: got % X, want % X", tt.name, got, tt.want)
		}
	}
	if _, err := EncodePropValue(DataType(0x00FF), PropValue{}); err == nil {
		t.Error("expected an error for an unknown datatype")
	}
}

func TestEncodePropValueRoundTrip(t *testing.T) {
	for _, dt := range []DataType{DTInt8, DTUint8, DTInt16, DTUint16, DTInt32, DTUint32, DTInt64, DTUint64} {
		want := int64(-5)
		if !dt.Signed() {
			want = 5
		}
		enc, err := EncodePropValue(dt, PropValue{Raw: want})
		if err != nil {
			t.Fatalf("encode 0x%04X: %v", uint16(dt), err)
		}
		d := decoder{buf: enc}
		got := propValue(&d, dt, "value")
		if d.err != nil {
			t.Fatalf("decode 0x%04X: %v", uint16(dt), d.err)
		}
		if got.Raw != want {
			t.Errorf("0x%04X: round trip %d != %d", uint16(dt), got.Raw, want)
		}
	}
}
