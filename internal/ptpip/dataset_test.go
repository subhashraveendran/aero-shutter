package ptpip

import (
	"testing"
	"time"
)

func TestPTPStringRoundTrip(t *testing.T) {
	cases := []string{"", "DSC_0042.NEF", "20240101T120000", "café ☕"}
	for _, want := range cases {
		enc := EncodePTPString(want)
		got, n, err := DecodePTPString(enc)
		if err != nil {
			t.Fatalf("DecodePTPString(%q): %v", want, err)
		}
		if got != want {
			t.Errorf("round trip = %q, want %q", got, want)
		}
		if n != len(enc) {
			t.Errorf("consumed %d of %d bytes for %q", n, len(enc), want)
		}
	}
}

func TestPTPStringTruncated(t *testing.T) {
	// Claims 5 chars but supplies only 2 bytes.
	if _, _, err := DecodePTPString([]byte{5, 'a', 0}); err == nil {
		t.Fatal("expected error for truncated PTP string")
	}
}

func TestParsePTPDateTime(t *testing.T) {
	got := ParsePTPDateTime("20240315T142530")
	want := time.Date(2024, 3, 15, 14, 25, 30, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("ParsePTPDateTime = %v, want %v", got, want)
	}
	if !ParsePTPDateTime("").IsZero() {
		t.Error("empty datetime should be zero")
	}
	if !ParsePTPDateTime("garbage").IsZero() {
		t.Error("garbage datetime should be zero")
	}
}

func TestObjectInfoRoundTrip(t *testing.T) {
	in := ObjectInfo{
		StorageID:       0x00010001,
		Format:          FormatJPEG,
		CompressedSize:  12_345_678,
		ThumbFormat:     FormatJPEG,
		ThumbSize:       11_000,
		ThumbWidth:      160,
		ThumbHeight:     120,
		ImageWidth:      6000,
		ImageHeight:     4000,
		ParentObject:    0x42,
		Filename:        "DSC_0042.JPG",
		CaptureDateRaw:  "20240315T142530",
		ModificationRaw: "20240315T142530",
	}
	out, err := DecodeObjectInfo(EncodeObjectInfo(in))
	if err != nil {
		t.Fatalf("DecodeObjectInfo: %v", err)
	}
	if out.Filename != in.Filename {
		t.Errorf("filename = %q, want %q", out.Filename, in.Filename)
	}
	if out.Format != FormatJPEG {
		t.Errorf("format = %v, want JPEG", out.Format)
	}
	if out.CompressedSize != in.CompressedSize {
		t.Errorf("size = %d, want %d", out.CompressedSize, in.CompressedSize)
	}
	if out.StorageID != in.StorageID {
		t.Errorf("storage = %#x, want %#x", out.StorageID, in.StorageID)
	}
	want := time.Date(2024, 3, 15, 14, 25, 30, 0, time.Local)
	if !out.CaptureDate.Equal(want) {
		t.Errorf("capture date = %v, want %v", out.CaptureDate, want)
	}
}

func TestDecodeObjectInfoTruncated(t *testing.T) {
	full := EncodeObjectInfo(ObjectInfo{Filename: "X.JPG"})
	if _, err := DecodeObjectInfo(full[:10]); err == nil {
		t.Fatal("expected error for truncated ObjectInfo")
	}
}

func TestDecodeUint32Array(t *testing.T) {
	raw := []byte{
		3, 0, 0, 0,
		1, 0, 0, 0,
		2, 0, 0, 0,
		0xFF, 0xFF, 0xFF, 0xFF,
	}
	got, err := DecodeUint32Array(raw)
	if err != nil {
		t.Fatalf("DecodeUint32Array: %v", err)
	}
	want := []uint32{1, 2, 0xFFFFFFFF}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %d, want %d", i, got[i], want[i])
		}
	}
	if _, err := DecodeUint32Array(raw[:7]); err == nil {
		t.Error("expected error for truncated array")
	}
}
