package ptpip

import (
	"bytes"
	"testing"
)

func TestPacketRoundTrip(t *testing.T) {
	in := Packet{Type: PktOperationRequest, Payload: []byte{1, 2, 3, 4, 5}}
	var buf bytes.Buffer
	if err := WritePacket(&buf, in); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	out, err := ReadPacket(&buf)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if out.Type != in.Type {
		t.Errorf("type = %v, want %v", out.Type, in.Type)
	}
	if !bytes.Equal(out.Payload, in.Payload) {
		t.Errorf("payload = %v, want %v", out.Payload, in.Payload)
	}
}

func TestPacketRejectsBadLength(t *testing.T) {
	// Header claims length 4, below the 8-byte minimum.
	raw := []byte{4, 0, 0, 0, 6, 0, 0, 0}
	if _, err := ReadPacket(bytes.NewReader(raw)); err == nil {
		t.Fatal("expected error for undersized packet length")
	}
}

func TestInitCommandRequestEncoding(t *testing.T) {
	req := InitCommandRequest{Name: "aero-shutter", Version: ProtocolVersion10}
	copy(req.GUID[:], bytes.Repeat([]byte{0xAB}, 16))
	pkt := req.Encode()
	if pkt.Type != PktInitCommandRequest {
		t.Fatalf("type = %v", pkt.Type)
	}
	// GUID + "aero-shutter" UTF-16LE + null + version.
	wantLen := 16 + (len(req.Name)+1)*2 + 4
	if len(pkt.Payload) != wantLen {
		t.Fatalf("payload len = %d, want %d", len(pkt.Payload), wantLen)
	}
	if !bytes.Equal(pkt.Payload[:16], req.GUID[:]) {
		t.Error("GUID not at start of payload")
	}
	if pkt.Payload[16] != req.Name[0] || pkt.Payload[17] != 0 {
		t.Error("name is not UTF-16LE")
	}
}

func TestInitCommandAckRoundTrip(t *testing.T) {
	payload := []byte{0x2A, 0, 0, 0} // connection number 42
	payload = append(payload, bytes.Repeat([]byte{0xCD}, 16)...)
	payload = append(payload, encodeUTF16LEString("D5300")...)
	payload = append(payload, 0, 0, 1, 0) // version 1.0
	ack, err := DecodeInitCommandAck(Packet{Type: PktInitCommandAck, Payload: payload})
	if err != nil {
		t.Fatalf("DecodeInitCommandAck: %v", err)
	}
	if ack.ConnectionNumber != 42 {
		t.Errorf("connection number = %d, want 42", ack.ConnectionNumber)
	}
	if ack.Name != "D5300" {
		t.Errorf("name = %q, want D5300", ack.Name)
	}
	if ack.Version != ProtocolVersion10 {
		t.Errorf("version = %#x, want %#x", ack.Version, ProtocolVersion10)
	}
}

func TestOperationRequestResponseRoundTrip(t *testing.T) {
	req := OperationRequest{OpCode: OpGetPartialObject, TxnID: 7, Params: []uint32{0x100, 0, 1 << 20}}
	pkt := req.Encode()
	if pkt.Type != PktOperationRequest {
		t.Fatalf("type = %v", pkt.Type)
	}
	// dataPhase(4) + opcode(2) + txn(4) + 3 params.
	if len(pkt.Payload) != 4+2+4+12 {
		t.Fatalf("payload len = %d", len(pkt.Payload))
	}

	respPayload := []byte{0x01, 0x20, 7, 0, 0, 0, 0xEF, 0xBE, 0, 0}
	resp, err := DecodeOperationResponse(Packet{Type: PktOperationResponse, Payload: respPayload})
	if err != nil {
		t.Fatalf("DecodeOperationResponse: %v", err)
	}
	if resp.Code != RespOK || resp.TxnID != 7 {
		t.Errorf("resp = %+v", resp)
	}
	if len(resp.Params) != 1 || resp.Params[0] != 0xBEEF {
		t.Errorf("params = %v, want [0xBEEF]", resp.Params)
	}
}

func TestStartDataRoundTrip(t *testing.T) {
	in := StartData{TxnID: 9, TotalLength: 1<<32 + 5}
	out, err := DecodeStartData(in.Encode())
	if err != nil {
		t.Fatalf("DecodeStartData: %v", err)
	}
	if out != in {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}
