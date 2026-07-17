package ptpip

import (
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf16"
)

// maxPacketPayload bounds the size of a single PTP/IP packet payload we are
// willing to read into memory. Bulk object data is streamed and never held in
// a single buffer larger than this.
const maxPacketPayload = 16 << 20 // 16 MiB

// headerSize is the fixed PTP/IP packet header: uint32 length + uint32 type.
const headerSize = 8

// Packet is a raw PTP/IP packet: a type and its payload (excluding the
// 8-byte length/type header).
type Packet struct {
	Type    PacketType
	Payload []byte
}

// Encode serialises the packet with its little-endian length/type header.
func (p Packet) Encode() []byte {
	buf := make([]byte, headerSize+len(p.Payload))
	binary.LittleEndian.PutUint32(buf[0:4], uint32(headerSize+len(p.Payload)))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(p.Type))
	copy(buf[headerSize:], p.Payload)
	return buf
}

// WritePacket writes a single packet to w.
func WritePacket(w io.Writer, p Packet) error {
	_, err := w.Write(p.Encode())
	return err
}

// ReadPacket reads a single packet from r, validating the framing.
func ReadPacket(r io.Reader) (Packet, error) {
	var hdr [headerSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Packet{}, err
	}
	length := binary.LittleEndian.Uint32(hdr[0:4])
	ptype := PacketType(binary.LittleEndian.Uint32(hdr[4:8]))
	if length < headerSize {
		return Packet{}, fmt.Errorf("ptpip: invalid packet length %d", length)
	}
	payloadLen := length - headerSize
	if payloadLen > maxPacketPayload {
		return Packet{}, fmt.Errorf("ptpip: packet payload too large (%d bytes)", payloadLen)
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Packet{}, err
	}
	return Packet{Type: ptype, Payload: payload}, nil
}

// encodeUTF16LEString encodes s as UTF-16LE with a null terminator, the wire
// format used for the friendly name in InitCommandRequest/Ack.
func encodeUTF16LEString(s string) []byte {
	units := utf16.Encode([]rune(s))
	buf := make([]byte, 0, (len(units)+1)*2)
	for _, u := range units {
		buf = binary.LittleEndian.AppendUint16(buf, u)
	}
	return binary.LittleEndian.AppendUint16(buf, 0)
}

// decodeUTF16LEString decodes a null-terminated UTF-16LE string starting at
// b, returning the string and the number of bytes consumed.
func decodeUTF16LEString(b []byte) (string, int) {
	var units []uint16
	i := 0
	for i+1 < len(b) {
		u := binary.LittleEndian.Uint16(b[i : i+2])
		i += 2
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units)), i
}

// InitCommandRequest is the first packet sent on the command connection. It
// carries the client GUID, friendly name and protocol version.
type InitCommandRequest struct {
	GUID    [16]byte
	Name    string
	Version uint32 // 0x00010000 = 1.0
}

// ProtocolVersion10 is PTP/IP protocol version 1.0.
const ProtocolVersion10 uint32 = 0x00010000

// Encode serialises the InitCommandRequest into a Packet.
func (r InitCommandRequest) Encode() Packet {
	payload := make([]byte, 0, 16+len(r.Name)*2+2+4)
	payload = append(payload, r.GUID[:]...)
	payload = append(payload, encodeUTF16LEString(r.Name)...)
	payload = binary.LittleEndian.AppendUint32(payload, r.Version)
	return Packet{Type: PktInitCommandRequest, Payload: payload}
}

// InitCommandAck is the camera's reply to InitCommandRequest, carrying the
// connection number used to bind the event connection.
type InitCommandAck struct {
	ConnectionNumber uint32
	GUID             [16]byte
	Name             string
	Version          uint32
}

// DecodeInitCommandAck parses an InitCommandAck payload.
func DecodeInitCommandAck(p Packet) (InitCommandAck, error) {
	if p.Type != PktInitCommandAck {
		return InitCommandAck{}, fmt.Errorf("ptpip: expected InitCommandAck, got %s", p.Type)
	}
	if len(p.Payload) < 4+16 {
		return InitCommandAck{}, fmt.Errorf("ptpip: InitCommandAck payload too short (%d bytes)", len(p.Payload))
	}
	var ack InitCommandAck
	ack.ConnectionNumber = binary.LittleEndian.Uint32(p.Payload[0:4])
	copy(ack.GUID[:], p.Payload[4:20])
	rest := p.Payload[20:]
	name, n := decodeUTF16LEString(rest)
	ack.Name = name
	rest = rest[n:]
	if len(rest) >= 4 {
		ack.Version = binary.LittleEndian.Uint32(rest[0:4])
	}
	return ack, nil
}

// InitEventRequest binds the event connection to a command connection using
// the connection number from InitCommandAck.
type InitEventRequest struct {
	ConnectionNumber uint32
}

// Encode serialises the InitEventRequest into a Packet.
func (r InitEventRequest) Encode() Packet {
	payload := binary.LittleEndian.AppendUint32(nil, r.ConnectionNumber)
	return Packet{Type: PktInitEventRequest, Payload: payload}
}

// Data phase flags carried in an OperationRequest.
const (
	dataPhaseNone = 1 // no data or data-in (device to host)
	dataPhaseOut  = 2 // data-out (host to device)
)

// OperationRequest is a PTP operation request packet.
type OperationRequest struct {
	DataOut bool // true when the host will send a data phase
	OpCode  OpCode
	TxnID   uint32
	Params  []uint32 // up to 5
}

// Encode serialises the OperationRequest into a Packet.
func (r OperationRequest) Encode() Packet {
	phase := uint32(dataPhaseNone)
	if r.DataOut {
		phase = dataPhaseOut
	}
	payload := make([]byte, 0, 4+2+4+len(r.Params)*4)
	payload = binary.LittleEndian.AppendUint32(payload, phase)
	payload = binary.LittleEndian.AppendUint16(payload, uint16(r.OpCode))
	payload = binary.LittleEndian.AppendUint32(payload, r.TxnID)
	for _, p := range r.Params {
		payload = binary.LittleEndian.AppendUint32(payload, p)
	}
	return Packet{Type: PktOperationRequest, Payload: payload}
}

// OperationResponse is a PTP operation response packet.
type OperationResponse struct {
	Code   ResponseCode
	TxnID  uint32
	Params []uint32
}

// DecodeOperationResponse parses an OperationResponse payload.
func DecodeOperationResponse(p Packet) (OperationResponse, error) {
	if p.Type != PktOperationResponse {
		return OperationResponse{}, fmt.Errorf("ptpip: expected OperationResponse, got %s", p.Type)
	}
	if len(p.Payload) < 6 {
		return OperationResponse{}, fmt.Errorf("ptpip: OperationResponse payload too short (%d bytes)", len(p.Payload))
	}
	var resp OperationResponse
	resp.Code = ResponseCode(binary.LittleEndian.Uint16(p.Payload[0:2]))
	resp.TxnID = binary.LittleEndian.Uint32(p.Payload[2:6])
	rest := p.Payload[6:]
	for len(rest) >= 4 {
		resp.Params = append(resp.Params, binary.LittleEndian.Uint32(rest[0:4]))
		rest = rest[4:]
	}
	return resp, nil
}

// StartData announces a data phase of TotalLength bytes for a transaction.
type StartData struct {
	TxnID       uint32
	TotalLength uint64
}

// Encode serialises the StartData packet.
func (s StartData) Encode() Packet {
	payload := make([]byte, 0, 12)
	payload = binary.LittleEndian.AppendUint32(payload, s.TxnID)
	payload = binary.LittleEndian.AppendUint64(payload, s.TotalLength)
	return Packet{Type: PktStartData, Payload: payload}
}

// DecodeStartData parses a StartData payload.
func DecodeStartData(p Packet) (StartData, error) {
	if p.Type != PktStartData {
		return StartData{}, fmt.Errorf("ptpip: expected StartData, got %s", p.Type)
	}
	if len(p.Payload) < 12 {
		return StartData{}, fmt.Errorf("ptpip: StartData payload too short (%d bytes)", len(p.Payload))
	}
	return StartData{
		TxnID:       binary.LittleEndian.Uint32(p.Payload[0:4]),
		TotalLength: binary.LittleEndian.Uint64(p.Payload[4:12]),
	}, nil
}

// dataPacketPayload extracts the transaction ID and data chunk from a Data or
// EndData packet.
func dataPacketPayload(p Packet) (txnID uint32, data []byte, err error) {
	if p.Type != PktData && p.Type != PktEndData {
		return 0, nil, fmt.Errorf("ptpip: expected Data/EndData, got %s", p.Type)
	}
	if len(p.Payload) < 4 {
		return 0, nil, fmt.Errorf("ptpip: data packet payload too short (%d bytes)", len(p.Payload))
	}
	return binary.LittleEndian.Uint32(p.Payload[0:4]), p.Payload[4:], nil
}

// Event is a PTP event delivered on the event connection.
type Event struct {
	Code   uint16
	TxnID  uint32
	Params []uint32
}

// DecodeEvent parses an Event packet payload.
func DecodeEvent(p Packet) (Event, error) {
	if p.Type != PktEvent {
		return Event{}, fmt.Errorf("ptpip: expected Event, got %s", p.Type)
	}
	if len(p.Payload) < 6 {
		return Event{}, fmt.Errorf("ptpip: Event payload too short (%d bytes)", len(p.Payload))
	}
	var ev Event
	ev.Code = binary.LittleEndian.Uint16(p.Payload[0:2])
	ev.TxnID = binary.LittleEndian.Uint32(p.Payload[2:6])
	rest := p.Payload[6:]
	for len(rest) >= 4 {
		ev.Params = append(ev.Params, binary.LittleEndian.Uint32(rest[0:4]))
		rest = rest[4:]
	}
	return ev, nil
}
