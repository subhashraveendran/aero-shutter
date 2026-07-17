package ptpip

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// DefaultPort is the well-known PTP/IP TCP port.
const DefaultPort = 15740

// Timeouts applied to camera I/O. Cameras can be slow to answer the first
// request after waking their Wi-Fi radio, so these are deliberately generous.
const (
	dialTimeout  = 5 * time.Second
	ioTimeout    = 30 * time.Second
	probeTimeout = 2 * time.Second
)

// ErrNotConnected is returned when an operation is attempted on a closed or
// never-connected client.
var ErrNotConnected = errors.New("ptpip: not connected")

// Client is a PTP/IP client bound to a single camera. It owns the command
// and event TCP connections and serialises transactions.
type Client struct {
	mu        sync.Mutex
	cmdConn   net.Conn
	evtConn   net.Conn
	txnID     uint32
	sessionID uint32
	guid      [16]byte
	name      string

	eventsMu sync.Mutex
	events   []Event
	evtDone  chan struct{}
}

// NewClient creates a client with the given friendly name announced to the
// camera. A random GUID is generated per client.
func NewClient(friendlyName string) *Client {
	c := &Client{name: friendlyName}
	if _, err := rand.Read(c.guid[:]); err != nil {
		// crypto/rand never fails on supported platforms; fall back to a
		// fixed GUID rather than propagating an error from a constructor.
		copy(c.guid[:], []byte("aero-shutter-guid!"))
	}
	return c
}

// Connect dials the camera at addr (host or host:port), performs the PTP/IP
// initialisation handshake on both connections, and opens a PTP session.
func (c *Client) Connect(ctx context.Context, addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, fmt.Sprint(DefaultPort))
	}
	d := net.Dialer{Timeout: dialTimeout}

	cmdConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("ptpip: dial command connection: %w", err)
	}
	setKeepAlive(cmdConn)

	ack, err := c.initCommand(ctx, cmdConn)
	if err != nil {
		cmdConn.Close()
		return err
	}

	evtConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		cmdConn.Close()
		return fmt.Errorf("ptpip: dial event connection: %w", err)
	}
	setKeepAlive(evtConn)

	if err := c.initEvent(ctx, evtConn, ack.ConnectionNumber); err != nil {
		cmdConn.Close()
		evtConn.Close()
		return err
	}

	c.mu.Lock()
	c.cmdConn = cmdConn
	c.evtConn = evtConn
	c.txnID = 0
	c.mu.Unlock()

	c.evtDone = make(chan struct{})
	go c.readEvents(evtConn)

	if err := c.OpenSession(ctx, 1); err != nil {
		// Some firmware answers SessionAlreadyOpen after a reconnect; treat
		// it as success.
		if !IsPTPError(err, RespSessionAlreadyOpen) {
			c.Close()
			return err
		}
	}
	return nil
}

func setKeepAlive(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(15 * time.Second)
	}
}

func (c *Client) initCommand(ctx context.Context, conn net.Conn) (InitCommandAck, error) {
	deadline(ctx, conn)
	req := InitCommandRequest{GUID: c.guid, Name: c.name, Version: ProtocolVersion10}
	if err := WritePacket(conn, req.Encode()); err != nil {
		return InitCommandAck{}, fmt.Errorf("ptpip: send InitCommandRequest: %w", err)
	}
	pkt, err := ReadPacket(conn)
	if err != nil {
		return InitCommandAck{}, fmt.Errorf("ptpip: read InitCommandAck: %w", err)
	}
	if pkt.Type == PktInitFail {
		return InitCommandAck{}, errors.New("ptpip: camera rejected connection (InitFail); is another host connected?")
	}
	return DecodeInitCommandAck(pkt)
}

func (c *Client) initEvent(ctx context.Context, conn net.Conn, connNum uint32) error {
	deadline(ctx, conn)
	if err := WritePacket(conn, InitEventRequest{ConnectionNumber: connNum}.Encode()); err != nil {
		return fmt.Errorf("ptpip: send InitEventRequest: %w", err)
	}
	pkt, err := ReadPacket(conn)
	if err != nil {
		return fmt.Errorf("ptpip: read InitEventAck: %w", err)
	}
	if pkt.Type == PktInitFail {
		return errors.New("ptpip: camera rejected event connection (InitFail)")
	}
	if pkt.Type != PktInitEventAck {
		return fmt.Errorf("ptpip: expected InitEventAck, got %s", pkt.Type)
	}
	return nil
}

// readEvents drains the event connection in the background so the camera is
// never blocked writing events. Received events are buffered for Events().
func (c *Client) readEvents(conn net.Conn) {
	defer close(c.evtDone)
	for {
		_ = conn.SetReadDeadline(time.Time{})
		pkt, err := ReadPacket(conn)
		if err != nil {
			return
		}
		if pkt.Type != PktEvent {
			continue
		}
		ev, err := DecodeEvent(pkt)
		if err != nil {
			continue
		}
		c.eventsMu.Lock()
		if len(c.events) < 256 {
			c.events = append(c.events, ev)
		}
		c.eventsMu.Unlock()
	}
}

// Events returns and clears any buffered camera events.
func (c *Client) Events() []Event {
	c.eventsMu.Lock()
	defer c.eventsMu.Unlock()
	evs := c.events
	c.events = nil
	return evs
}

// Close closes both connections. It attempts a best-effort CloseSession first.
func (c *Client) Close() error {
	c.mu.Lock()
	cmdConn, evtConn := c.cmdConn, c.evtConn
	c.cmdConn, c.evtConn = nil, nil
	c.mu.Unlock()

	var err error
	if cmdConn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = c.closeSessionOn(ctx, cmdConn)
		cancel()
		err = cmdConn.Close()
	}
	if evtConn != nil {
		if e := evtConn.Close(); err == nil {
			err = e
		}
	}
	return err
}

func (c *Client) closeSessionOn(ctx context.Context, conn net.Conn) error {
	deadline(ctx, conn)
	req := OperationRequest{OpCode: OpCloseSession, TxnID: c.nextTxnID()}
	if err := WritePacket(conn, req.Encode()); err != nil {
		return err
	}
	_, err := ReadPacket(conn)
	return err
}

// Connected reports whether the command connection is established.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cmdConn != nil
}

func (c *Client) nextTxnID() uint32 {
	c.txnID++
	return c.txnID
}

func deadline(ctx context.Context, conn net.Conn) {
	dl := time.Now().Add(ioTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(dl) {
		dl = d
	}
	_ = conn.SetDeadline(dl)
}

// Transact runs a full PTP transaction: request, optional data-out, optional
// data-in streamed to dataIn, and response. dataOut and dataIn may be nil.
// It returns the OperationResponse; a non-OK response code yields a *PTPError.
func (c *Client) Transact(ctx context.Context, op OpCode, params []uint32, dataOut []byte, dataIn io.Writer) (OperationResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	conn := c.cmdConn
	if conn == nil {
		return OperationResponse{}, ErrNotConnected
	}
	if err := ctx.Err(); err != nil {
		return OperationResponse{}, err
	}

	txn := c.nextTxnID()
	deadline(ctx, conn)

	req := OperationRequest{DataOut: dataOut != nil, OpCode: op, TxnID: txn, Params: params}
	if err := WritePacket(conn, req.Encode()); err != nil {
		c.dropLocked()
		return OperationResponse{}, fmt.Errorf("ptpip: send %s request: %w", op.name(), err)
	}

	if dataOut != nil {
		if err := WritePacket(conn, StartData{TxnID: txn, TotalLength: uint64(len(dataOut))}.Encode()); err != nil {
			c.dropLocked()
			return OperationResponse{}, err
		}
		payload := append([]byte{0, 0, 0, 0}, dataOut...)
		putUint32(payload[0:4], txn)
		if err := WritePacket(conn, Packet{Type: PktEndData, Payload: payload}); err != nil {
			c.dropLocked()
			return OperationResponse{}, err
		}
	}

	for {
		if err := ctx.Err(); err != nil {
			c.dropLocked()
			return OperationResponse{}, err
		}
		deadline(ctx, conn)
		pkt, err := ReadPacket(conn)
		if err != nil {
			c.dropLocked()
			return OperationResponse{}, fmt.Errorf("ptpip: read during %s: %w", op.name(), err)
		}
		switch pkt.Type {
		case PktStartData:
			// Length announcement; nothing to do — data follows.
		case PktData, PktEndData:
			_, chunk, derr := dataPacketPayload(pkt)
			if derr != nil {
				c.dropLocked()
				return OperationResponse{}, derr
			}
			if dataIn != nil && len(chunk) > 0 {
				if _, werr := dataIn.Write(chunk); werr != nil {
					c.dropLocked()
					return OperationResponse{}, fmt.Errorf("ptpip: write data phase: %w", werr)
				}
			}
		case PktOperationResponse:
			resp, derr := DecodeOperationResponse(pkt)
			if derr != nil {
				c.dropLocked()
				return OperationResponse{}, derr
			}
			if resp.Code != RespOK {
				return resp, &PTPError{Op: op, Code: resp.Code}
			}
			return resp, nil
		default:
			// Ignore stray packets (e.g. probe requests answered below).
			if pkt.Type == PktProbeRequest {
				_ = WritePacket(conn, Packet{Type: PktProbeResponse})
			}
		}
	}
}

// dropLocked marks the connection dead after an I/O error. Caller holds mu.
func (c *Client) dropLocked() {
	if c.cmdConn != nil {
		c.cmdConn.Close()
		c.cmdConn = nil
	}
	if c.evtConn != nil {
		c.evtConn.Close()
		c.evtConn = nil
	}
}

func putUint32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func (op OpCode) name() string { return fmt.Sprintf("0x%04X", uint16(op)) }

// OpenSession opens a PTP session with the given session ID.
func (c *Client) OpenSession(ctx context.Context, sessionID uint32) error {
	_, err := c.Transact(ctx, OpOpenSession, []uint32{sessionID}, nil, nil)
	if err == nil {
		c.sessionID = sessionID
	}
	return err
}

// CloseSession closes the current PTP session.
func (c *Client) CloseSession(ctx context.Context) error {
	_, err := c.Transact(ctx, OpCloseSession, nil, nil, nil)
	return err
}

// GetDeviceInfo fetches and parses the DeviceInfo dataset.
func (c *Client) GetDeviceInfo(ctx context.Context) (DeviceInfo, error) {
	var buf memBuffer
	if _, err := c.Transact(ctx, OpGetDeviceInfo, nil, nil, &buf); err != nil {
		return DeviceInfo{}, err
	}
	return DecodeDeviceInfo(buf.b)
}

// GetStorageIDs fetches the list of storage IDs.
func (c *Client) GetStorageIDs(ctx context.Context) ([]uint32, error) {
	var buf memBuffer
	if _, err := c.Transact(ctx, OpGetStorageIDs, nil, nil, &buf); err != nil {
		return nil, err
	}
	return DecodeUint32Array(buf.b)
}

// GetStorageInfo fetches and parses the StorageInfo dataset for a storage ID.
func (c *Client) GetStorageInfo(ctx context.Context, storageID uint32) (StorageInfo, error) {
	var buf memBuffer
	if _, err := c.Transact(ctx, OpGetStorageInfo, []uint32{storageID}, nil, &buf); err != nil {
		return StorageInfo{}, err
	}
	return DecodeStorageInfo(buf.b)
}

// AllStorages is the wildcard storage ID matching every store.
const AllStorages uint32 = 0xFFFFFFFF

// AllFormats matches every object format in GetObjectHandles.
const AllFormats uint32 = 0

// RootParent selects objects in the root of a store; 0 selects all objects.
const RootParent uint32 = 0xFFFFFFFF

// GetObjectHandles lists object handles on a storage, optionally filtered by
// format code and parent association handle.
func (c *Client) GetObjectHandles(ctx context.Context, storageID, formatCode, parent uint32) ([]uint32, error) {
	var buf memBuffer
	if _, err := c.Transact(ctx, OpGetObjectHandles, []uint32{storageID, formatCode, parent}, nil, &buf); err != nil {
		return nil, err
	}
	return DecodeUint32Array(buf.b)
}

// GetObjectInfo fetches and parses the ObjectInfo dataset for a handle.
func (c *Client) GetObjectInfo(ctx context.Context, handle uint32) (ObjectInfo, error) {
	var buf memBuffer
	if _, err := c.Transact(ctx, OpGetObjectInfo, []uint32{handle}, nil, &buf); err != nil {
		return ObjectInfo{}, err
	}
	return DecodeObjectInfo(buf.b)
}

// GetThumb fetches the embedded thumbnail (JPEG) for a handle.
func (c *Client) GetThumb(ctx context.Context, handle uint32) ([]byte, error) {
	var buf memBuffer
	if _, err := c.Transact(ctx, OpGetThumb, []uint32{handle}, nil, &buf); err != nil {
		return nil, err
	}
	return buf.b, nil
}

// GetObject streams the full object for a handle to w.
func (c *Client) GetObject(ctx context.Context, handle uint32, w io.Writer) error {
	_, err := c.Transact(ctx, OpGetObject, []uint32{handle}, nil, w)
	return err
}

// GetPartialObject streams up to maxBytes of the object starting at offset
// to w, returning the number of bytes actually transferred.
func (c *Client) GetPartialObject(ctx context.Context, handle uint32, offset, maxBytes uint32, w io.Writer) (uint32, error) {
	cw := &countingWriter{w: w}
	resp, err := c.Transact(ctx, OpGetPartialObject, []uint32{handle, offset, maxBytes}, nil, cw)
	if err != nil {
		return uint32(cw.n), err
	}
	if len(resp.Params) > 0 {
		return resp.Params[0], nil
	}
	return uint32(cw.n), nil
}

// GetDevicePropValue fetches the raw value of a device property.
func (c *Client) GetDevicePropValue(ctx context.Context, prop DevicePropCode) ([]byte, error) {
	var buf memBuffer
	if _, err := c.Transact(ctx, OpGetDevicePropVal, []uint32{uint32(prop)}, nil, &buf); err != nil {
		return nil, err
	}
	return buf.b, nil
}

// BatteryLevel reads DevicePropCode 0x5001 and returns the level as a
// percentage in 0-100.
func (c *Client) BatteryLevel(ctx context.Context) (int, error) {
	raw, err := c.GetDevicePropValue(ctx, PropBatteryLevel)
	if err != nil {
		return 0, err
	}
	if len(raw) == 0 {
		return 0, errors.New("ptpip: empty battery level value")
	}
	return int(raw[0]), nil
}

// memBuffer is a minimal growable buffer used for small datasets so that we
// avoid pulling bytes.Buffer's extra bookkeeping into the hot path.
type memBuffer struct{ b []byte }

func (m *memBuffer) Write(p []byte) (int, error) {
	m.b = append(m.b, p...)
	return len(p), nil
}

// countingWriter wraps a writer and counts bytes written through it.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// Probe checks whether a PTP/IP responder listens at addr within timeout. It
// only completes a TCP handshake; it does not disturb existing sessions.
func Probe(ctx context.Context, addr string, timeout time.Duration) bool {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, fmt.Sprint(DefaultPort))
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
