package ptpip

import (
	"context"
	"encoding/binary"
	"fmt"
)

// DataType is a PTP device property datatype code.
type DataType uint16

// PTP datatype codes (ISO 15740 §5.3).
const (
	DTInt8   DataType = 0x0001
	DTUint8  DataType = 0x0002
	DTInt16  DataType = 0x0003
	DTUint16 DataType = 0x0004
	DTInt32  DataType = 0x0005
	DTUint32 DataType = 0x0006
	DTInt64  DataType = 0x0007
	DTUint64 DataType = 0x0008
	DTStr    DataType = 0xFFFF
)

// Signed reports whether the datatype holds a signed integer.
func (t DataType) Signed() bool {
	switch t {
	case DTInt8, DTInt16, DTInt32, DTInt64:
		return true
	}
	return false
}

// Form flags of the DevicePropDesc dataset.
const (
	FormNone  uint8 = 0
	FormRange uint8 = 1
	FormEnum  uint8 = 2
)

// PropValue holds one device property value. Integer types (signed and
// unsigned) are carried in Raw as a sign-extended int64; string properties
// use Str.
type PropValue struct {
	Raw int64
	Str string
}

// PropRange describes the range form of a DevicePropDesc: the value moves
// between Min and Max in increments of Step.
type PropRange struct {
	Min, Max, Step int64
}

// DevicePropDesc is the parsed DevicePropDesc dataset (GetDevicePropDesc,
// operation 0x1014) describing one device property.
type DevicePropDesc struct {
	Code     DevicePropCode
	DataType DataType
	Writable bool // GetSet flag == 1
	Factory  PropValue
	Current  PropValue
	FormFlag uint8
	Range    *PropRange  // set when FormFlag == FormRange
	Enum     []PropValue // set when FormFlag == FormEnum
}

// propValue reads one value of the given datatype from the decoder.
func propValue(d *decoder, dt DataType, what string) PropValue {
	switch dt {
	case DTInt8:
		return PropValue{Raw: int64(int8(d.u8(what)))}
	case DTUint8:
		return PropValue{Raw: int64(d.u8(what))}
	case DTInt16:
		return PropValue{Raw: int64(int16(d.u16(what)))}
	case DTUint16:
		return PropValue{Raw: int64(d.u16(what))}
	case DTInt32:
		return PropValue{Raw: int64(int32(d.u32(what)))}
	case DTUint32:
		return PropValue{Raw: int64(d.u32(what))}
	case DTInt64, DTUint64:
		return PropValue{Raw: int64(d.u64(what))}
	case DTStr:
		return PropValue{Str: d.str(what)}
	default:
		if d.err == nil {
			d.err = fmt.Errorf("ptpip: unsupported datatype 0x%04X reading %s", uint16(dt), what)
		}
		return PropValue{}
	}
}

// DecodeDevicePropDesc parses a DevicePropDesc dataset payload.
func DecodeDevicePropDesc(b []byte) (DevicePropDesc, error) {
	d := decoder{buf: b}
	var pd DevicePropDesc
	pd.Code = DevicePropCode(d.u16("DevicePropertyCode"))
	pd.DataType = DataType(d.u16("DataType"))
	pd.Writable = d.u8("GetSet") == 1
	pd.Factory = propValue(&d, pd.DataType, "FactoryDefaultValue")
	pd.Current = propValue(&d, pd.DataType, "CurrentValue")
	pd.FormFlag = d.u8("FormFlag")
	switch pd.FormFlag {
	case FormRange:
		r := PropRange{
			Min:  propValue(&d, pd.DataType, "MinimumValue").Raw,
			Max:  propValue(&d, pd.DataType, "MaximumValue").Raw,
			Step: propValue(&d, pd.DataType, "StepSize").Raw,
		}
		pd.Range = &r
	case FormEnum:
		n := int(d.u16("NumberOfValues"))
		if d.err == nil {
			pd.Enum = make([]PropValue, 0, n)
			for i := 0; i < n; i++ {
				pd.Enum = append(pd.Enum, propValue(&d, pd.DataType, "SupportedValue"))
			}
		}
	}
	if d.err != nil {
		return DevicePropDesc{}, d.err
	}
	return pd, nil
}

// EncodePropValue serialises a property value in the given datatype, as sent
// in the data phase of SetDevicePropValue.
func EncodePropValue(dt DataType, v PropValue) ([]byte, error) {
	switch dt {
	case DTInt8, DTUint8:
		return []byte{byte(v.Raw)}, nil
	case DTInt16, DTUint16:
		return binary.LittleEndian.AppendUint16(nil, uint16(v.Raw)), nil
	case DTInt32, DTUint32:
		return binary.LittleEndian.AppendUint32(nil, uint32(v.Raw)), nil
	case DTInt64, DTUint64:
		return binary.LittleEndian.AppendUint64(nil, uint64(v.Raw)), nil
	case DTStr:
		return EncodePTPString(v.Str), nil
	default:
		return nil, fmt.Errorf("ptpip: cannot encode datatype 0x%04X", uint16(dt))
	}
}

// GetDevicePropDesc fetches and parses the DevicePropDesc dataset for a
// device property.
func (c *Client) GetDevicePropDesc(ctx context.Context, prop DevicePropCode) (DevicePropDesc, error) {
	var buf memBuffer
	if _, err := c.Transact(ctx, OpGetDevicePropDesc, []uint32{uint32(prop)}, nil, &buf); err != nil {
		return DevicePropDesc{}, err
	}
	return DecodeDevicePropDesc(buf.b)
}

// SetDevicePropValue writes a new value for a device property, encoded in the
// property's datatype.
func (c *Client) SetDevicePropValue(ctx context.Context, prop DevicePropCode, dt DataType, v PropValue) error {
	data, err := EncodePropValue(dt, v)
	if err != nil {
		return err
	}
	_, err = c.Transact(ctx, OpSetDevicePropVal, []uint32{uint32(prop)}, data, nil)
	return err
}

// InitiateCapture asks the camera to take a picture using the default storage
// and format (both parameters zero, per ISO 15740).
func (c *Client) InitiateCapture(ctx context.Context) error {
	_, err := c.Transact(ctx, OpInitiateCapture, []uint32{0, 0}, nil, nil)
	return err
}
