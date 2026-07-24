// PTP/IP packet types (little-endian framing: {u32 len, u32 type, payload}).
export const PacketType = {
  InitCommandRequest: 1,
  InitCommandAck: 2,
  InitEventRequest: 3,
  InitEventAck: 4,
  InitFail: 5,
  OperationRequest: 6,
  OperationResponse: 7,
  Event: 8,
  StartData: 9,
  Data: 10,
  Cancel: 11,
  EndData: 12,
  ProbeRequest: 13,
  ProbeResponse: 14,
} as const;

export type PacketTypeValue = (typeof PacketType)[keyof typeof PacketType];

// Data phase indicator for OperationRequest.
export const DataPhase = {
  NoData: 1,
  DataToInitiator: 2, // read (camera -> host)
  DataToResponder: 3, // write (host -> camera)
} as const;

// Standard PTP operation codes.
export const OpCode = {
  GetDeviceInfo: 0x1001,
  OpenSession: 0x1002,
  CloseSession: 0x1003,
  GetStorageIDs: 0x1004,
  GetStorageInfo: 0x1005,
  GetNumObjects: 0x1006,
  GetObjectHandles: 0x1007,
  GetObjectInfo: 0x1008,
  GetObject: 0x1009,
  GetThumb: 0x100a,
  DeleteObject: 0x100b,
  InitiateCapture: 0x100e,
  GetDevicePropDesc: 0x1014,
  GetDevicePropValue: 0x1015,
  SetDevicePropValue: 0x1016,
  GetPartialObject: 0x101b,
  // Nikon vendor extension: large thumbnail.
  NikonGetLargeThumb: 0x90c4,
  // Nikon vendor extensions: "Send to smart device" transfer queue.
  // SetTransferListLock param = lock mode (3 = lock, 0 = unlock). GetTransferList
  // returns the AUINT32 array of object handles marked on the camera body.
  NikonSetTransferListLock: 0x9407,
  NikonGetTransferList: 0x9408,
} as const;

// Transfer-list lock modes for NikonSetTransferListLock (match Nikon WMU usage).
export const TransferListLock = 3;
export const TransferListUnlock = 0;

// PTP response codes.
export const RespCode = {
  OK: 0x2001,
  GeneralError: 0x2002,
  SessionNotOpen: 0x2003,
  OperationNotSupported: 0x2005,
  ParameterNotSupported: 0x2006,
  DevicePropNotSupported: 0x200a,
  InvalidObjectHandle: 0x2009,
  AccessDenied: 0x200f,
} as const;

// PTP/IP event codes (carried in Event packets on the event channel).
export const EventCode = {
  ObjectAdded: 0x4002,
  ObjectRemoved: 0x4003,
  StoreAdded: 0x4004,
  StoreRemoved: 0x4005,
  DevicePropChanged: 0x4006,
  StoreFull: 0x400a,
  CaptureComplete: 0x400d,
} as const;

export type EventCodeValue = (typeof EventCode)[keyof typeof EventCode];

// PTP datatype codes used in DevicePropDesc.
export const DataType = {
  INT8: 0x0001,
  UINT8: 0x0002,
  INT16: 0x0003,
  UINT16: 0x0004,
  INT32: 0x0005,
  UINT32: 0x0006,
  INT64: 0x0007,
  UINT64: 0x0008,
  STR: 0xffff,
} as const;

// Device property codes we care about.
export const PropCode = {
  BatteryLevel: 0x5001,
  ImageSize: 0x5003,
  WhiteBalance: 0x5005,
  FNumber: 0x5007,
  FocusMode: 0x500a,
  ExposureMeteringMode: 0x500b,
  ExposureProgramMode: 0x500e,
  ExposureIndex: 0x500f, // ISO
  ExposureBiasCompensation: 0x5010, // EV
  ExposureTime: 0x500d, // shutter
} as const;

// Object format codes.
export const ObjectFormat = {
  EXIF_JPEG: 0x3801,
  TIFF_EP: 0x3802, // NEF raw is reported here by Nikon
  NEF: 0x3800,
} as const;

// PTP/IP protocol version 1.0 (major 1, minor 0) packed as u32.
export const PTPIP_VERSION = 0x00010000;

// Default TCP port for PTP/IP.
export const PTPIP_PORT = 15740;
