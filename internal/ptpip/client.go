package ptpip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// DefaultPort is the well-known PTP/IP TCP port.
const DefaultPort = 15740

// tcpRecvBuffer is the SO_RCVBUF size requested on both connections. PTP/IP
// throughput over Wi-Fi is capped by the TCP receive window (bandwidth-delay
// product); a small default buffer leaves the link idle between chunks. Nikon's
// own Wireless Mobile Utility enlarges this in native code (setTcpRecvBuf); we
// do the same, sized generously for 802.11n/ac links.
const tcpRecvBuffer = 4 << 20 // 4 MiB

// WMUInitiatorGUID is the fixed 16-byte initiator GUID announced in the PTP/IP
// Init_Command_Request. Nikon's Wireless Mobile Utility hardcodes this exact
// value; sending a stable GUID (rather than a random one per connection) lets
// the camera recognise us as a returning host and skip re-pairing prompts.
var WMUInitiatorGUID = [16]byte{
	0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
	0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
}

// Timeouts applied to camera I/O. Cameras can be slow to answer the first
// request after waking their Wi-Fi radio, so these are deliberately generous.
const (
	// DefaultDialTimeout is used when Client.DialTimeout is left at zero. It
	// matches Nikon's Wireless Mobile Utility (10s) so slow-to-wake radios have
	// enough time to answer the first SYN.
	DefaultDialTimeout = 10 * time.Second
	ioTimeout          = 30 * time.Second
	probeTimeout       = 2 * time.Second
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

	// DialTimeout overrides DefaultDialTimeout for Connect when non-zero.
	DialTimeout time.Duration

	eventsMu sync.Mutex
	events   []Event
	evtDone  chan struct{}

	// handlerMu guards the dispatch handlers and the per-connection
	// disconnect-once latch.
	handlerMu    sync.Mutex
	eventHandler func(Event)
	disconnectFn func(error)
	disconnected *sync.Once
}

// NewClient creates a client with the given friendly name announced to the
// camera. The initiator GUID is fixed (WMUInitiatorGUID), matching Nikon's
// Wireless Mobile Utility, so the camera treats us as the same returning host
// on every connection.
func NewClient(friendlyName string) *Client {
	return &Client{name: friendlyName, guid: WMUInitiatorGUID}
}

// SetEventHandler registers fn to be called for every decoded camera event as
// it arrives on the event connection. Events continue to be buffered for
// Events() regardless (back-compat). Pass nil to unregister. The handler runs
// on the event-reader goroutine, so it must not block for long.
func (c *Client) SetEventHandler(fn func(Event)) {
	c.handlerMu.Lock()
	c.eventHandler = fn
	c.handlerMu.Unlock()
}

// SetDisconnectHandler registers fn to be called exactly once per connection
// the first time the link is observed dead — either the event socket read
// fails or a transaction hits a dead socket. Pass nil to unregister.
func (c *Client) SetDisconnectHandler(fn func(error)) {
	c.handlerMu.Lock()
	c.disconnectFn = fn
	c.handlerMu.Unlock()
}

// fireDisconnect invokes the registered disconnect handler at most once for the
// current connection.
func (c *Client) fireDisconnect(err error) {
	c.handlerMu.Lock()
	once := c.disconnected
	fn := c.disconnectFn
	c.handlerMu.Unlock()
	if once == nil || fn == nil {
		return
	}
	once.Do(func() { fn(err) })
}

// Connect dials the camera at addr (host or host:port), performs the PTP/IP
// initialisation handshake on both connections, and opens a PTP session.
func (c *Client) Connect(ctx context.Context, addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, fmt.Sprint(DefaultPort))
	}
	dt := c.DialTimeout
	if dt <= 0 {
		dt = DefaultDialTimeout
	}
	d := net.Dialer{Timeout: dt}

	cmdConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("ptpip: dial command connection: %w", err)
	}
	tuneSocket(cmdConn)

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
	tuneSocket(evtConn)

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

	// Arm a fresh disconnect latch for this connection so the disconnect
	// handler fires exactly once, even if both the event loop and a
	// transaction observe the dead socket.
	c.handlerMu.Lock()
	c.disconnected = &sync.Once{}
	c.handlerMu.Unlock()

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

// tuneSocket applies the socket options that matter for PTP/IP over Wi-Fi:
// keep-alive (so an idle camera doesn't drop us), TCP_NODELAY (the request path
// is a stream of small packets that must not wait on Nagle), and an enlarged
// receive buffer (the throughput lever — mirrors WMU's setTcpRecvBuf).
func tuneSocket(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(15 * time.Second)
		_ = tc.SetNoDelay(true)
		_ = tc.SetReadBuffer(tcpRecvBuffer)
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
			// The event socket died. If this is not the result of a
			// deliberate Close (which nils cmdConn), surface it as a
			// disconnect so the app can reconnect immediately rather than
			// waiting for the next user operation to fail.
			c.mu.Lock()
			deliberate := c.cmdConn == nil
			c.mu.Unlock()
			if !deliberate {
				c.fireDisconnect(fmt.Errorf("ptpip: event connection lost: %w", err))
			}
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

		// Dispatch to the registered handler (in addition to buffering).
		c.handlerMu.Lock()
		fn := c.eventHandler
		c.handlerMu.Unlock()
		if fn != nil {
			fn(ev)
		}
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

	resp, err := c.transactLocked(ctx, op, params, dataOut, dataIn)

	// Session re-open recovery: some firmware forgets the session after an
	// idle drop-and-restore and answers SessionNotOpen. Transparently reopen
	// the session and retry the transaction once before failing. OpenSession
	// itself is excluded to avoid recursion.
	if err != nil && op != OpOpenSession && IsPTPError(err, RespSessionNotOpen) && c.cmdConn != nil {
		sid := c.sessionID
		if sid == 0 {
			sid = 1
		}
		if _, oerr := c.transactLocked(ctx, OpOpenSession, []uint32{sid}, nil, nil); oerr == nil {
			c.sessionID = sid
			// GetObject-style data writers may have received partial data on
			// the first attempt; callers of Transact with a dataIn writer must
			// tolerate a retry from offset zero, which mirrors existing chunked
			// download behaviour (each chunk is an independent transaction).
			return c.transactLocked(ctx, op, params, dataOut, dataIn)
		}
	}
	return resp, err
}

// transactLocked performs a single PTP transaction. Caller holds mu.
func (c *Client) transactLocked(ctx context.Context, op OpCode, params []uint32, dataOut []byte, dataIn io.Writer) (OperationResponse, error) {
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
		err = fmt.Errorf("ptpip: send %s request: %w", op.name(), err)
		c.dropLocked(err)
		return OperationResponse{}, err
	}

	if dataOut != nil {
		if err := WritePacket(conn, StartData{TxnID: txn, TotalLength: uint64(len(dataOut))}.Encode()); err != nil {
			c.dropLocked(err)
			return OperationResponse{}, err
		}
		payload := append([]byte{0, 0, 0, 0}, dataOut...)
		putUint32(payload[0:4], txn)
		if err := WritePacket(conn, Packet{Type: PktEndData, Payload: payload}); err != nil {
			c.dropLocked(err)
			return OperationResponse{}, err
		}
	}

	for {
		if err := ctx.Err(); err != nil {
			c.dropLocked(err)
			return OperationResponse{}, err
		}
		deadline(ctx, conn)
		pkt, err := ReadPacket(conn)
		if err != nil {
			err = fmt.Errorf("ptpip: read during %s: %w", op.name(), err)
			c.dropLocked(err)
			return OperationResponse{}, err
		}
		switch pkt.Type {
		case PktStartData:
			// Length announcement; nothing to do — data follows.
		case PktData, PktEndData:
			_, chunk, derr := dataPacketPayload(pkt)
			if derr != nil {
				c.dropLocked(derr)
				return OperationResponse{}, derr
			}
			if dataIn != nil && len(chunk) > 0 {
				if _, werr := dataIn.Write(chunk); werr != nil {
					werr = fmt.Errorf("ptpip: write data phase: %w", werr)
					c.dropLocked(werr)
					return OperationResponse{}, werr
				}
			}
		case PktOperationResponse:
			resp, derr := DecodeOperationResponse(pkt)
			if derr != nil {
				c.dropLocked(derr)
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

// dropLocked marks the connection dead after an I/O error and fires the
// disconnect handler (once per connection) so the app can react immediately.
// Caller holds mu.
func (c *Client) dropLocked(cause error) {
	if c.cmdConn != nil {
		c.cmdConn.Close()
		c.cmdConn = nil
	}
	if c.evtConn != nil {
		c.evtConn.Close()
		c.evtConn = nil
	}
	// fireDisconnect locks handlerMu only (not mu), so calling it while holding
	// mu is safe and cannot deadlock.
	if cause == nil {
		cause = errors.New("ptpip: connection lost")
	}
	c.fireDisconnect(cause)
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

// KeepAlive performs a cheap PTP round-trip (GetStorageIDs) to keep the link
// warm and to detect a dead socket while otherwise idle. A failure fires the
// disconnect handler through the normal drop path, so callers can treat a
// non-nil return as "connection lost".
func (c *Client) KeepAlive(ctx context.Context) error {
	_, err := c.GetStorageIDs(ctx)
	return err
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

// GetLargeThumb fetches a larger JPEG preview for a handle using the Nikon
// vendor operation 0x90C4. Bodies that do not implement it answer with a PTP
// error (typically OperationNotSupported); callers should then fall back to
// GetThumb.
func (c *Client) GetLargeThumb(ctx context.Context, handle uint32) ([]byte, error) {
	var buf memBuffer
	if _, err := c.Transact(ctx, OpNikonGetLargeThumb, []uint32{handle}, nil, &buf); err != nil {
		return nil, err
	}
	return buf.b, nil
}

// Transfer-list lock modes for SetTransferListLock, matching Nikon WMU usage.
const (
	TransferListUnlock uint32 = 0
	TransferListLock   uint32 = 3
)

// SetTransferListLock locks (mode=TransferListLock) or unlocks
// (mode=TransferListUnlock) the camera's "Send to smart device" transfer queue
// via Nikon vendor operation 0x9407, so the app can read the marked list without
// the camera mutating it. Unsupported bodies answer a PTP error.
func (c *Client) SetTransferListLock(ctx context.Context, mode uint32) error {
	_, err := c.Transact(ctx, OpNikonSetTransferListLock, []uint32{mode}, nil, nil)
	return err
}

// GetTransferList returns the object handles the user marked on the camera body
// for "Send to smart device", via Nikon vendor operation 0x9408. The data phase
// is a standard PTP AUINT32 handle array. Unsupported bodies answer a PTP error.
func (c *Client) GetTransferList(ctx context.Context) ([]uint32, error) {
	var buf memBuffer
	if _, err := c.Transact(ctx, OpNikonGetTransferList, nil, nil, &buf); err != nil {
		return nil, err
	}
	if len(buf.b) == 0 {
		return nil, nil
	}
	return DecodeUint32Array(buf.b)
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
