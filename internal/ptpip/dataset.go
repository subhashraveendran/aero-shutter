package ptpip

import (
	"encoding/binary"
	"fmt"
	"time"
	"unicode/utf16"
)

// decoder is a cursor over a PTP dataset payload with little-endian helpers.
type decoder struct {
	buf []byte
	off int
	err error
}

func (d *decoder) remaining() int { return len(d.buf) - d.off }

func (d *decoder) fail(what string) {
	if d.err == nil {
		d.err = fmt.Errorf("ptpip: truncated dataset reading %s at offset %d", what, d.off)
	}
}

func (d *decoder) u8(what string) uint8 {
	if d.err != nil || d.remaining() < 1 {
		d.fail(what)
		return 0
	}
	v := d.buf[d.off]
	d.off++
	return v
}

func (d *decoder) u16(what string) uint16 {
	if d.err != nil || d.remaining() < 2 {
		d.fail(what)
		return 0
	}
	v := binary.LittleEndian.Uint16(d.buf[d.off:])
	d.off += 2
	return v
}

func (d *decoder) u32(what string) uint32 {
	if d.err != nil || d.remaining() < 4 {
		d.fail(what)
		return 0
	}
	v := binary.LittleEndian.Uint32(d.buf[d.off:])
	d.off += 4
	return v
}

func (d *decoder) u64(what string) uint64 {
	if d.err != nil || d.remaining() < 8 {
		d.fail(what)
		return 0
	}
	v := binary.LittleEndian.Uint64(d.buf[d.off:])
	d.off += 8
	return v
}

// str reads a PTP string: uint8 character count (including the null
// terminator) followed by that many UTF-16LE code units.
func (d *decoder) str(what string) string {
	n := int(d.u8(what))
	if d.err != nil {
		return ""
	}
	if n == 0 {
		return ""
	}
	if d.remaining() < n*2 {
		d.fail(what)
		return ""
	}
	units := make([]uint16, 0, n)
	for i := 0; i < n; i++ {
		u := binary.LittleEndian.Uint16(d.buf[d.off:])
		d.off += 2
		if u == 0 {
			continue
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units))
}

// u16Array reads a PTP array of uint16: uint32 count followed by elements.
func (d *decoder) u16Array(what string) []uint16 {
	n := int(d.u32(what))
	if d.err != nil {
		return nil
	}
	if n < 0 || d.remaining() < n*2 {
		d.fail(what)
		return nil
	}
	out := make([]uint16, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint16(d.buf[d.off:])
		d.off += 2
	}
	return out
}

// EncodePTPString serialises s as a PTP string (uint8 length including the
// null terminator, UTF-16LE code units, null terminated). Empty strings
// encode as a single zero byte.
func EncodePTPString(s string) []byte {
	if s == "" {
		return []byte{0}
	}
	units := utf16.Encode([]rune(s))
	if len(units) > 254 {
		units = units[:254]
	}
	buf := make([]byte, 0, 1+(len(units)+1)*2)
	buf = append(buf, byte(len(units)+1))
	for _, u := range units {
		buf = binary.LittleEndian.AppendUint16(buf, u)
	}
	return binary.LittleEndian.AppendUint16(buf, 0)
}

// DecodePTPString parses a PTP string at the start of b, returning the string
// and bytes consumed.
func DecodePTPString(b []byte) (string, int, error) {
	d := decoder{buf: b}
	s := d.str("string")
	if d.err != nil {
		return "", 0, d.err
	}
	return s, d.off, nil
}

// ParsePTPDateTime parses a PTP datetime string ("YYYYMMDDThhmmss" with an
// optional ".s" tenth and optional timezone suffix). It returns the zero time
// when the string is empty or unparseable.
func ParsePTPDateTime(s string) time.Time {
	if len(s) < 15 {
		return time.Time{}
	}
	base := s[:15]
	t, err := time.ParseInLocation("20060102T150405", base, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ObjectInfo is the PTP ObjectInfo dataset for a single object handle.
type ObjectInfo struct {
	StorageID        uint32
	Format           ObjectFormat
	ProtectionStatus uint16
	CompressedSize   uint32
	ThumbFormat      ObjectFormat
	ThumbSize        uint32
	ThumbWidth       uint32
	ThumbHeight      uint32
	ImageWidth       uint32
	ImageHeight      uint32
	ImageBitDepth    uint32
	ParentObject     uint32
	AssociationType  uint16
	AssociationDesc  uint32
	SequenceNumber   uint32
	Filename         string
	CaptureDate      time.Time
	ModificationDate time.Time
	CaptureDateRaw   string
	ModificationRaw  string
	Keywords         string
}

// DecodeObjectInfo parses an ObjectInfo dataset payload.
func DecodeObjectInfo(b []byte) (ObjectInfo, error) {
	d := decoder{buf: b}
	var oi ObjectInfo
	oi.StorageID = d.u32("StorageID")
	oi.Format = ObjectFormat(d.u16("ObjectFormat"))
	oi.ProtectionStatus = d.u16("ProtectionStatus")
	oi.CompressedSize = d.u32("CompressedSize")
	oi.ThumbFormat = ObjectFormat(d.u16("ThumbFormat"))
	oi.ThumbSize = d.u32("ThumbSize")
	oi.ThumbWidth = d.u32("ThumbWidth")
	oi.ThumbHeight = d.u32("ThumbHeight")
	oi.ImageWidth = d.u32("ImageWidth")
	oi.ImageHeight = d.u32("ImageHeight")
	oi.ImageBitDepth = d.u32("ImageBitDepth")
	oi.ParentObject = d.u32("ParentObject")
	oi.AssociationType = d.u16("AssociationType")
	oi.AssociationDesc = d.u32("AssociationDesc")
	oi.SequenceNumber = d.u32("SequenceNumber")
	oi.Filename = d.str("Filename")
	oi.CaptureDateRaw = d.str("CaptureDate")
	oi.ModificationRaw = d.str("ModificationDate")
	oi.Keywords = d.str("Keywords")
	if d.err != nil {
		return ObjectInfo{}, d.err
	}
	oi.CaptureDate = ParsePTPDateTime(oi.CaptureDateRaw)
	oi.ModificationDate = ParsePTPDateTime(oi.ModificationRaw)
	return oi, nil
}

// EncodeObjectInfo serialises an ObjectInfo dataset. It is primarily used by
// tests to verify round-trip parsing.
func EncodeObjectInfo(oi ObjectInfo) []byte {
	buf := make([]byte, 0, 64+len(oi.Filename)*2)
	buf = binary.LittleEndian.AppendUint32(buf, oi.StorageID)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(oi.Format))
	buf = binary.LittleEndian.AppendUint16(buf, oi.ProtectionStatus)
	buf = binary.LittleEndian.AppendUint32(buf, oi.CompressedSize)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(oi.ThumbFormat))
	buf = binary.LittleEndian.AppendUint32(buf, oi.ThumbSize)
	buf = binary.LittleEndian.AppendUint32(buf, oi.ThumbWidth)
	buf = binary.LittleEndian.AppendUint32(buf, oi.ThumbHeight)
	buf = binary.LittleEndian.AppendUint32(buf, oi.ImageWidth)
	buf = binary.LittleEndian.AppendUint32(buf, oi.ImageHeight)
	buf = binary.LittleEndian.AppendUint32(buf, oi.ImageBitDepth)
	buf = binary.LittleEndian.AppendUint32(buf, oi.ParentObject)
	buf = binary.LittleEndian.AppendUint16(buf, oi.AssociationType)
	buf = binary.LittleEndian.AppendUint32(buf, oi.AssociationDesc)
	buf = binary.LittleEndian.AppendUint32(buf, oi.SequenceNumber)
	buf = append(buf, EncodePTPString(oi.Filename)...)
	buf = append(buf, EncodePTPString(oi.CaptureDateRaw)...)
	buf = append(buf, EncodePTPString(oi.ModificationRaw)...)
	buf = append(buf, EncodePTPString(oi.Keywords)...)
	return buf
}

// StorageInfo is the PTP StorageInfo dataset for a storage ID.
type StorageInfo struct {
	StorageType        uint16
	FilesystemType     uint16
	AccessCapability   uint16
	MaxCapacity        uint64
	FreeSpaceInBytes   uint64
	FreeSpaceInObjects uint32
	Description        string
	VolumeLabel        string
}

// DecodeStorageInfo parses a StorageInfo dataset payload.
func DecodeStorageInfo(b []byte) (StorageInfo, error) {
	d := decoder{buf: b}
	var si StorageInfo
	si.StorageType = d.u16("StorageType")
	si.FilesystemType = d.u16("FilesystemType")
	si.AccessCapability = d.u16("AccessCapability")
	si.MaxCapacity = d.u64("MaxCapacity")
	si.FreeSpaceInBytes = d.u64("FreeSpaceInBytes")
	si.FreeSpaceInObjects = d.u32("FreeSpaceInObjects")
	si.Description = d.str("StorageDescription")
	si.VolumeLabel = d.str("VolumeLabel")
	if d.err != nil {
		return StorageInfo{}, d.err
	}
	return si, nil
}

// DeviceInfo is a subset of the PTP DeviceInfo dataset.
type DeviceInfo struct {
	StandardVersion     uint16
	VendorExtensionID   uint32
	VendorExtensionVer  uint16
	VendorExtensionDesc string
	FunctionalMode      uint16
	OperationsSupported []uint16
	EventsSupported     []uint16
	DevicePropsSupport  []uint16
	CaptureFormats      []uint16
	ImageFormats        []uint16
	Manufacturer        string
	Model               string
	DeviceVersion       string
	SerialNumber        string
}

// DecodeDeviceInfo parses a DeviceInfo dataset payload.
func DecodeDeviceInfo(b []byte) (DeviceInfo, error) {
	d := decoder{buf: b}
	var di DeviceInfo
	di.StandardVersion = d.u16("StandardVersion")
	di.VendorExtensionID = d.u32("VendorExtensionID")
	di.VendorExtensionVer = d.u16("VendorExtensionVersion")
	di.VendorExtensionDesc = d.str("VendorExtensionDesc")
	di.FunctionalMode = d.u16("FunctionalMode")
	di.OperationsSupported = d.u16Array("OperationsSupported")
	di.EventsSupported = d.u16Array("EventsSupported")
	di.DevicePropsSupport = d.u16Array("DevicePropertiesSupported")
	di.CaptureFormats = d.u16Array("CaptureFormats")
	di.ImageFormats = d.u16Array("ImageFormats")
	di.Manufacturer = d.str("Manufacturer")
	di.Model = d.str("Model")
	di.DeviceVersion = d.str("DeviceVersion")
	di.SerialNumber = d.str("SerialNumber")
	if d.err != nil {
		return DeviceInfo{}, d.err
	}
	return di, nil
}

// SupportsOperation reports whether the device advertises the given opcode.
func (di DeviceInfo) SupportsOperation(op OpCode) bool {
	for _, o := range di.OperationsSupported {
		if o == uint16(op) {
			return true
		}
	}
	return false
}

// DecodeUint32Array parses a PTP array of uint32 (uint32 count + elements),
// used for storage ID and object handle lists.
func DecodeUint32Array(b []byte) ([]uint32, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("ptpip: uint32 array too short (%d bytes)", len(b))
	}
	n := int(binary.LittleEndian.Uint32(b))
	if len(b) < 4+n*4 {
		return nil, fmt.Errorf("ptpip: uint32 array truncated (want %d elements, have %d bytes)", n, len(b)-4)
	}
	out := make([]uint32, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(b[4+i*4:])
	}
	return out, nil
}
