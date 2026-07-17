// Package ptpip implements the PTP/IP transport (ISO 15740 over TCP) used by
// Wi-Fi capable Nikon cameras such as the D5300. It provides packet framing,
// the connection initialisation handshake, and command/data/response
// transactions on top of two TCP connections to port 15740.
package ptpip

import "fmt"

// PacketType identifies a PTP/IP packet on the wire.
type PacketType uint32

// PTP/IP packet types as defined by the PTP-IP specification.
const (
	PktInvalid            PacketType = 0
	PktInitCommandRequest PacketType = 1
	PktInitCommandAck     PacketType = 2
	PktInitEventRequest   PacketType = 3
	PktInitEventAck       PacketType = 4
	PktInitFail           PacketType = 5
	PktOperationRequest   PacketType = 6
	PktOperationResponse  PacketType = 7
	PktEvent              PacketType = 8
	PktStartData          PacketType = 9
	PktData               PacketType = 10
	PktCancelTransaction  PacketType = 11
	PktEndData            PacketType = 12
	PktProbeRequest       PacketType = 13
	PktProbeResponse      PacketType = 14
)

// String returns a human-readable name for the packet type.
func (t PacketType) String() string {
	switch t {
	case PktInitCommandRequest:
		return "InitCommandRequest"
	case PktInitCommandAck:
		return "InitCommandAck"
	case PktInitEventRequest:
		return "InitEventRequest"
	case PktInitEventAck:
		return "InitEventAck"
	case PktInitFail:
		return "InitFail"
	case PktOperationRequest:
		return "OperationRequest"
	case PktOperationResponse:
		return "OperationResponse"
	case PktEvent:
		return "Event"
	case PktStartData:
		return "StartData"
	case PktData:
		return "Data"
	case PktCancelTransaction:
		return "CancelTransaction"
	case PktEndData:
		return "EndData"
	case PktProbeRequest:
		return "ProbeRequest"
	case PktProbeResponse:
		return "ProbeResponse"
	default:
		return fmt.Sprintf("PacketType(%d)", uint32(t))
	}
}

// OpCode is a PTP operation code.
type OpCode uint16

// PTP operation codes used by aero-shutter.
const (
	OpGetDeviceInfo     OpCode = 0x1001
	OpOpenSession       OpCode = 0x1002
	OpCloseSession      OpCode = 0x1003
	OpGetStorageIDs     OpCode = 0x1004
	OpGetStorageInfo    OpCode = 0x1005
	OpGetObjectHandles  OpCode = 0x1007
	OpGetObjectInfo     OpCode = 0x1008
	OpGetObject         OpCode = 0x1009
	OpGetThumb          OpCode = 0x100A
	OpInitiateCapture   OpCode = 0x100E
	OpGetDevicePropDesc OpCode = 0x1014
	OpGetDevicePropVal  OpCode = 0x1015
	OpSetDevicePropVal  OpCode = 0x1016
	OpGetPartialObject  OpCode = 0x101B

	// OpNikonGetLargeThumb is the Nikon vendor operation returning a larger
	// (~640px) JPEG preview than the standard GetThumb thumbnail. Not every
	// body supports it; callers must fall back to OpGetThumb on a PTP error.
	OpNikonGetLargeThumb OpCode = 0x90C4
)

// ResponseCode is a PTP response code returned in an OperationResponse.
type ResponseCode uint16

// Common PTP response codes.
const (
	RespOK                    ResponseCode = 0x2001
	RespGeneralError          ResponseCode = 0x2002
	RespSessionNotOpen        ResponseCode = 0x2003
	RespInvalidTransactionID  ResponseCode = 0x2004
	RespOperationNotSupported ResponseCode = 0x2005
	RespParameterNotSupported ResponseCode = 0x2006
	RespIncompleteTransfer    ResponseCode = 0x2007
	RespInvalidStorageID      ResponseCode = 0x2008
	RespInvalidObjectHandle   ResponseCode = 0x2009
	RespDevicePropNotSupport  ResponseCode = 0x200A
	RespInvalidObjectFormat   ResponseCode = 0x200B
	RespStoreFull             ResponseCode = 0x200C
	RespStoreNotAvailable     ResponseCode = 0x2013
	RespAccessDenied          ResponseCode = 0x200F
	RespDeviceBusy            ResponseCode = 0x2019
	RespInvalidParameter      ResponseCode = 0x201D
	RespSessionAlreadyOpen    ResponseCode = 0x201E
)

// String returns a human-readable name for the response code.
func (c ResponseCode) String() string {
	switch c {
	case RespOK:
		return "OK"
	case RespGeneralError:
		return "GeneralError"
	case RespSessionNotOpen:
		return "SessionNotOpen"
	case RespInvalidTransactionID:
		return "InvalidTransactionID"
	case RespOperationNotSupported:
		return "OperationNotSupported"
	case RespParameterNotSupported:
		return "ParameterNotSupported"
	case RespIncompleteTransfer:
		return "IncompleteTransfer"
	case RespInvalidStorageID:
		return "InvalidStorageID"
	case RespInvalidObjectHandle:
		return "InvalidObjectHandle"
	case RespDevicePropNotSupport:
		return "DevicePropNotSupported"
	case RespInvalidObjectFormat:
		return "InvalidObjectFormatCode"
	case RespStoreFull:
		return "StoreFull"
	case RespStoreNotAvailable:
		return "StoreNotAvailable"
	case RespAccessDenied:
		return "AccessDenied"
	case RespDeviceBusy:
		return "DeviceBusy"
	case RespInvalidParameter:
		return "InvalidParameter"
	case RespSessionAlreadyOpen:
		return "SessionAlreadyOpen"
	default:
		return fmt.Sprintf("ResponseCode(0x%04X)", uint16(c))
	}
}

// ObjectFormat is a PTP object format code.
type ObjectFormat uint16

// Object format codes relevant to Nikon cameras.
const (
	FormatAssociation ObjectFormat = 0x3001 // folder
	FormatUndefined   ObjectFormat = 0x3000 // NEF appears as undefined on some firmware
	FormatNEF         ObjectFormat = 0xB101 // Nikon raw
	FormatJPEG        ObjectFormat = 0x3801 // EXIF/JPEG
	FormatMOV         ObjectFormat = 0x300D // QuickTime movie
)

// String returns a short badge-style name for the format.
func (f ObjectFormat) String() string {
	switch f {
	case FormatAssociation:
		return "DIR"
	case FormatUndefined, FormatNEF:
		return "NEF"
	case FormatJPEG:
		return "JPG"
	case FormatMOV:
		return "MOV"
	default:
		return fmt.Sprintf("0x%04X", uint16(f))
	}
}

// IsImage reports whether the format is a still image (raw or JPEG).
func (f ObjectFormat) IsImage() bool {
	return f == FormatJPEG || f == FormatNEF || f == FormatUndefined
}

// DevicePropCode is a PTP device property code.
type DevicePropCode uint16

// Device property codes used by aero-shutter.
const (
	PropBatteryLevel        DevicePropCode = 0x5001
	PropWhiteBalance        DevicePropCode = 0x5005
	PropFNumber             DevicePropCode = 0x5007
	PropExposureTime        DevicePropCode = 0x500D
	PropExposureProgramMode DevicePropCode = 0x500E
	PropExposureIndex       DevicePropCode = 0x500F
	PropExposureBias        DevicePropCode = 0x5010
	PropStillCaptureMode    DevicePropCode = 0x5013
)

// PTPError is an error carrying a PTP response code.
type PTPError struct {
	Op   OpCode
	Code ResponseCode
}

// Error implements the error interface.
func (e *PTPError) Error() string {
	return fmt.Sprintf("ptp: operation 0x%04X failed: %s", uint16(e.Op), e.Code)
}

// IsPTPError reports whether err is a *PTPError with the given code.
func IsPTPError(err error, code ResponseCode) bool {
	pe, ok := err.(*PTPError)
	return ok && pe.Code == code
}
