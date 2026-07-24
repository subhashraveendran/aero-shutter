package camera

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
)

// HostVersion is the aero-shutter version announced to the camera as part of the
// PTP/IP friendly name. It defaults to "dev" and is overwritten by main at
// startup from the build-stamped version.
var HostVersion = "dev"

// friendlyName builds the initiator name sent in Init_Command_Request, matching
// the shape Nikon's Wireless Mobile Utility uses ("App/Version (OS x)").
func friendlyName() string {
	return fmt.Sprintf("aero-shutter/%s (%s)", HostVersion, runtime.GOOS)
}

// File describes a downloadable object on the camera.
type File struct {
	Handle      uint32
	StorageID   uint32
	Name        string
	Format      ptpip.ObjectFormat
	Size        int64
	CaptureTime time.Time
}

// Storage summarises a camera storage card.
type Storage struct {
	ID          uint32
	Description string
	Capacity    uint64
	Free        uint64
}

// Info identifies the connected camera.
type Info struct {
	Manufacturer string
	Model        string
	Serial       string
	Version      string
}

// DefaultKeepAliveInterval is the idle keep-alive period used when
// StartKeepAlive is given a non-positive interval.
const DefaultKeepAliveInterval = 30 * time.Second

// reconnectBackoff is the exponential backoff schedule (seconds) used by the
// auto-reconnect loop; it caps at the final value.
var reconnectBackoff = []time.Duration{
	1 * time.Second, 2 * time.Second, 4 * time.Second,
	8 * time.Second, 16 * time.Second, 30 * time.Second,
}

// ReconnectState describes an auto-reconnect status update surfaced to the UI.
type ReconnectState struct {
	// Attempt is the 1-based attempt counter.
	Attempt int
	// Waiting is the backoff delay before this attempt starts.
	Waiting time.Duration
	// Connected is true once a reconnect attempt succeeds.
	Connected bool
	// GaveUp is true when the loop stopped without connecting (cancelled).
	GaveUp bool
	// Err carries the last attempt error, when any.
	Err error
}

// Camera wraps a ptpip.Client with a capability profile and high-level
// operations tailored to photo import.
type Camera struct {
	client  *ptpip.Client
	profile Profile
	info    Info
	addr    string

	// largeThumbFailed remembers, per session, that the body rejected the
	// Nikon GetLargeThumb vendor operation so it is never retried.
	largeThumbFailed atomic.Bool

	// hookMu guards the app-level callbacks.
	hookMu       sync.Mutex
	onEvent      func(ptpip.Event)
	onDisconnect func(error)
	onReconnect  func(ReconnectState)

	// keep-alive / reconnect lifecycle.
	lifeMu       sync.Mutex
	keepAliveCtx context.CancelFunc
	reconnectCtx context.CancelFunc
}

// New creates a disconnected Camera.
func New() *Camera {
	c := &Camera{client: ptpip.NewClient(friendlyName()), profile: GenericProfile}
	// Bridge the low-level ptpip callbacks up to the app-level hooks. The
	// event bridge also powers instant auto-import on ObjectAdded.
	c.client.SetEventHandler(func(ev ptpip.Event) {
		c.hookMu.Lock()
		fn := c.onEvent
		c.hookMu.Unlock()
		if fn != nil {
			fn(ev)
		}
	})
	c.client.SetDisconnectHandler(func(err error) {
		c.hookMu.Lock()
		fn := c.onDisconnect
		c.hookMu.Unlock()
		if fn != nil {
			fn(err)
		}
	})
	return c
}

// SetDialTimeout overrides the TCP dial timeout used by Connect. A
// non-positive value restores the client default.
func (c *Camera) SetDialTimeout(d time.Duration) {
	if d < 0 {
		d = 0
	}
	c.client.DialTimeout = d
}

// OnEvent registers a callback invoked for every decoded camera event (e.g.
// ObjectAdded). Pass nil to unregister. The callback runs on the event-reader
// goroutine and must not block for long.
func (c *Camera) OnEvent(fn func(ptpip.Event)) {
	c.hookMu.Lock()
	c.onEvent = fn
	c.hookMu.Unlock()
}

// OnDisconnect registers a callback fired once when the link is observed dead
// (background event-socket failure or a transaction on a dead socket). Pass nil
// to unregister.
func (c *Camera) OnDisconnect(fn func(error)) {
	c.hookMu.Lock()
	c.onDisconnect = fn
	c.hookMu.Unlock()
}

// OnReconnect registers a callback that receives auto-reconnect status updates.
// Pass nil to unregister.
func (c *Camera) OnReconnect(fn func(ReconnectState)) {
	c.hookMu.Lock()
	c.onReconnect = fn
	c.hookMu.Unlock()
}

func (c *Camera) emitReconnect(s ReconnectState) {
	c.hookMu.Lock()
	fn := c.onReconnect
	c.hookMu.Unlock()
	if fn != nil {
		fn(s)
	}
}

// Connect dials the camera at addr, performs the PTP/IP handshake, reads
// DeviceInfo, and selects the matching capability profile.
func (c *Camera) Connect(ctx context.Context, addr string) error {
	if err := c.client.Connect(ctx, addr); err != nil {
		return err
	}
	di, err := c.client.GetDeviceInfo(ctx)
	if err != nil {
		c.client.Close()
		return fmt.Errorf("camera: read device info: %w", err)
	}
	c.addr = addr
	c.info = Info{
		Manufacturer: di.Manufacturer,
		Model:        di.Model,
		Serial:       di.SerialNumber,
		Version:      di.DeviceVersion,
	}
	c.profile = ProfileForModel(di.Model)
	if c.profile.SupportsPartialObject && !di.SupportsOperation(ptpip.OpGetPartialObject) {
		// Trust the camera's advertised operation set over the profile.
		c.profile.SupportsPartialObject = false
	}
	c.largeThumbFailed.Store(false) // new session: allow a fresh attempt
	return nil
}

// Disconnect closes the PTP session and both TCP connections. It also stops any
// running keep-alive loop and cancels an in-flight auto-reconnect, so an
// explicit user disconnect wins over automatic recovery.
func (c *Camera) Disconnect() error {
	c.StopKeepAlive()
	c.CancelReconnect()
	return c.client.Close()
}

// StartKeepAlive launches a background goroutine that performs a cheap PTP
// round-trip every interval while the camera is connected and idle, detecting a
// dead link promptly. A non-positive interval selects DefaultKeepAliveInterval.
// It is safe to call repeatedly; a previous loop is stopped first. Keep-alive
// failures flow through the client's disconnect handler (fired by the dead
// socket), which the frontend routes into auto-reconnect.
func (c *Camera) StartKeepAlive(interval time.Duration) {
	if interval <= 0 {
		interval = DefaultKeepAliveInterval
	}
	c.StopKeepAlive()

	ctx, cancel := context.WithCancel(context.Background())
	c.lifeMu.Lock()
	c.keepAliveCtx = cancel
	c.lifeMu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !c.client.Connected() {
					return
				}
				rctx, rcancel := context.WithTimeout(ctx, 15*time.Second)
				err := c.client.KeepAlive(rctx)
				rcancel()
				if err != nil {
					// The failing round-trip already dropped the socket and
					// fired the disconnect handler (which drives reconnect);
					// nothing more to do here.
					return
				}
			}
		}
	}()
}

// StopKeepAlive stops the keep-alive loop if one is running.
func (c *Camera) StopKeepAlive() {
	c.lifeMu.Lock()
	cancel := c.keepAliveCtx
	c.keepAliveCtx = nil
	c.lifeMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Reconnect runs an auto-reconnect loop against the last connected address with
// exponential backoff (1,2,4,8,16,30,30… capped at 30s), surfacing status via
// the OnReconnect callback. It stops on the first successful reconnect or when
// cancelled (CancelReconnect / Disconnect / ctx). Only one loop runs at a time;
// a fresh call replaces any in-flight loop. It returns immediately; progress is
// reported through the callback.
func (c *Camera) Reconnect(interval time.Duration) {
	addr := c.addr
	if addr == "" {
		return
	}
	// Replace any in-flight loop so loops never stack.
	c.CancelReconnect()

	ctx, cancel := context.WithCancel(context.Background())
	c.lifeMu.Lock()
	c.reconnectCtx = cancel
	c.lifeMu.Unlock()

	go func() {
		defer func() {
			c.lifeMu.Lock()
			if c.reconnectCtx != nil {
				// Clear our own cancel if it is still the active one.
				c.reconnectCtx = nil
			}
			c.lifeMu.Unlock()
		}()

		for attempt := 1; ; attempt++ {
			wait := backoffFor(attempt)
			c.emitReconnect(ReconnectState{Attempt: attempt, Waiting: wait})

			select {
			case <-ctx.Done():
				c.emitReconnect(ReconnectState{Attempt: attempt, GaveUp: true})
				return
			case <-time.After(wait):
			}

			// Ensure any half-open sockets are gone before redialing.
			_ = c.client.Close()

			cctx, ccancel := context.WithTimeout(ctx, 45*time.Second)
			err := c.Connect(cctx, addr)
			ccancel()
			if err == nil {
				if interval <= 0 {
					interval = DefaultKeepAliveInterval
				}
				c.StartKeepAlive(interval)
				c.emitReconnect(ReconnectState{Attempt: attempt, Connected: true})
				return
			}
			if ctx.Err() != nil {
				c.emitReconnect(ReconnectState{Attempt: attempt, GaveUp: true})
				return
			}
			c.emitReconnect(ReconnectState{Attempt: attempt, Err: err})
		}
	}()
}

// CancelReconnect stops an in-flight auto-reconnect loop, if any.
func (c *Camera) CancelReconnect() {
	c.lifeMu.Lock()
	cancel := c.reconnectCtx
	c.reconnectCtx = nil
	c.lifeMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// backoffFor returns the backoff delay for a 1-based attempt number, capped at
// the final entry of reconnectBackoff.
func backoffFor(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	idx := attempt - 1
	if idx >= len(reconnectBackoff) {
		idx = len(reconnectBackoff) - 1
	}
	return reconnectBackoff[idx]
}

// Connected reports whether the camera link is up.
func (c *Camera) Connected() bool { return c.client.Connected() }

// Addr returns the address the camera was connected on.
func (c *Camera) Addr() string { return c.addr }

// DeviceInfo returns the identity of the connected camera.
func (c *Camera) DeviceInfo() Info { return c.info }

// Profile returns the active capability profile.
func (c *Camera) Profile() Profile { return c.profile }

// BatteryLevel returns the battery charge percentage (0-100).
func (c *Camera) BatteryLevel(ctx context.Context) (int, error) {
	return c.client.BatteryLevel(ctx)
}

// StorageInfo returns information about every storage card in the camera.
func (c *Camera) StorageInfo(ctx context.Context) ([]Storage, error) {
	ids, err := c.client.GetStorageIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Storage, 0, len(ids))
	for _, id := range ids {
		si, err := c.client.GetStorageInfo(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, Storage{
			ID:          id,
			Description: si.Description,
			Capacity:    si.MaxCapacity,
			Free:        si.FreeSpaceInBytes,
		})
	}
	return out, nil
}

// ListFiles walks every storage and returns all downloadable files (images
// and movies), newest first. Associations (folders) are skipped; handle
// enumeration uses the flat "all objects" query and filters by format.
func (c *Camera) ListFiles(ctx context.Context) ([]File, error) {
	ids, err := c.client.GetStorageIDs(ctx)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, sid := range ids {
		handles, err := c.client.GetObjectHandles(ctx, sid, ptpip.AllFormats, 0)
		if err != nil {
			return nil, fmt.Errorf("camera: list handles on storage 0x%08X: %w", sid, err)
		}
		for _, h := range handles {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			oi, err := c.client.GetObjectInfo(ctx, h)
			if err != nil {
				continue
			}
			if oi.Format == ptpip.FormatAssociation {
				continue
			}
			files = append(files, File{
				Handle:      h,
				StorageID:   sid,
				Name:        oi.Filename,
				Format:      oi.Format,
				Size:        int64(oi.CompressedSize),
				CaptureTime: oi.CaptureDate,
			})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if !files[i].CaptureTime.Equal(files[j].CaptureTime) {
			return files[i].CaptureTime.After(files[j].CaptureTime)
		}
		return files[i].Handle > files[j].Handle
	})
	return files, nil
}

// TransferList returns the photos the user marked on the camera body for
// "Send to smart device" (Nikon vendor ops 0x9407 lock / 0x9408 read). It locks
// the queue so the camera can't mutate it, resolves each handle to a File, then
// unlocks. Bodies that do not implement the vendor ops return (nil, nil) so
// callers can treat "unsupported" and "empty queue" identically — this is the
// camera-initiated counterpart to ObjectAdded auto-import.
func (c *Camera) TransferList(ctx context.Context) ([]File, error) {
	if err := c.client.SetTransferListLock(ctx, ptpip.TransferListLock); err != nil {
		var pe *ptpip.PTPError
		if errors.As(err, &pe) {
			return nil, nil // vendor op unsupported: no camera-marked queue
		}
		return nil, err
	}
	// Best-effort unlock even on error paths so we don't leave the camera locked.
	defer func() { _ = c.client.SetTransferListLock(ctx, ptpip.TransferListUnlock) }()

	handles, err := c.client.GetTransferList(ctx)
	if err != nil {
		var pe *ptpip.PTPError
		if errors.As(err, &pe) {
			return nil, nil
		}
		return nil, err
	}

	var files []File
	for _, h := range handles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		oi, err := c.client.GetObjectInfo(ctx, h)
		if err != nil {
			continue
		}
		if oi.Format == ptpip.FormatAssociation {
			continue
		}
		files = append(files, File{
			Handle:      h,
			Name:        oi.Filename,
			Format:      oi.Format,
			Size:        int64(oi.CompressedSize),
			CaptureTime: oi.CaptureDate,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		if !files[i].CaptureTime.Equal(files[j].CaptureTime) {
			return files[i].CaptureTime.After(files[j].CaptureTime)
		}
		return files[i].Handle > files[j].Handle
	})
	return files, nil
}

// GetThumb returns the best available JPEG preview for a handle, or nil when
// the profile disables thumbnails. When the profile enables LargeThumb the
// Nikon GetLargeThumb vendor operation (0x90C4) is tried first; a PTP error
// (e.g. OperationNotSupported) marks it unavailable for the rest of the
// session and the standard GetThumb (0x100A) is used instead.
func (c *Camera) GetThumb(ctx context.Context, handle uint32) ([]byte, error) {
	if c.profile.Thumbnails == ThumbNone {
		return nil, nil
	}
	return fetchThumb(ctx, handle, c.profile.LargeThumb, &c.largeThumbFailed,
		c.client.GetLargeThumb, c.client.GetThumb)
}

// thumbFetchFunc fetches preview bytes for an object handle.
type thumbFetchFunc func(ctx context.Context, handle uint32) ([]byte, error)

// fetchThumb implements the LargeThumb fallback state machine: try the large
// variant while it is enabled and not yet known to fail, permanently disable
// it for the session on a PTP error, and propagate transport errors (which
// say nothing about opcode support) unchanged.
func fetchThumb(ctx context.Context, handle uint32, tryLarge bool, failed *atomic.Bool, large, std thumbFetchFunc) ([]byte, error) {
	if tryLarge && !failed.Load() {
		data, err := large(ctx, handle)
		if err == nil {
			return data, nil
		}
		var pe *ptpip.PTPError
		if !errors.As(err, &pe) {
			return nil, err
		}
		failed.Store(true)
	}
	return std(ctx, handle)
}

// ProgressFunc receives the total number of bytes written so far (including
// any resumed offset).
type ProgressFunc func(written int64)

// DownloadTo streams the object to w starting at offset (for resume),
// invoking progress as bytes arrive. When the profile supports
// GetPartialObject the transfer proceeds in profile.ChunkSize chunks;
// otherwise the whole object is streamed with GetObject (offset must be 0).
func (c *Camera) DownloadTo(ctx context.Context, w io.Writer, f File, offset int64, progress ProgressFunc) error {
	if offset < 0 {
		return fmt.Errorf("camera: negative offset %d", offset)
	}
	if !c.profile.SupportsPartialObject {
		if offset != 0 {
			return fmt.Errorf("camera: %s does not support resume (GetPartialObject unavailable)", c.profile.Name)
		}
		pw := &progressWriter{w: w, fn: progress}
		return c.client.GetObject(ctx, f.Handle, pw)
	}

	chunk := c.profile.ChunkSize
	if chunk == 0 {
		chunk = 1 << 20
	}
	written := offset
	for written < f.Size {
		if err := ctx.Err(); err != nil {
			return err
		}
		want := chunk
		if remaining := f.Size - written; int64(want) > remaining {
			want = uint32(remaining)
		}
		pw := &progressWriter{w: w, fn: progress, base: written}
		n, err := c.client.GetPartialObject(ctx, f.Handle, uint32(written), want, pw)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("camera: zero-length chunk at offset %d for %s", written, f.Name)
		}
		written += int64(n)
	}
	return nil
}

// progressWriter forwards writes and reports cumulative progress.
type progressWriter struct {
	w    io.Writer
	fn   ProgressFunc
	base int64
	n    int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.n += int64(n)
	if p.fn != nil && n > 0 {
		p.fn(p.base + p.n)
	}
	return n, err
}

// localIPv4s returns the IPv4 addresses of up interfaces, used to derive /24
// scan ranges.
func localIPv4s() []net.IP {
	var out []net.IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 != nil {
				out = append(out, ip4)
			}
		}
	}
	return out
}
