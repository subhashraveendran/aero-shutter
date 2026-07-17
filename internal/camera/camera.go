package camera

import (
	"context"
	"fmt"
	"io"
	"net"
	"sort"
	"time"

	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
)

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

// Camera wraps a ptpip.Client with a capability profile and high-level
// operations tailored to photo import.
type Camera struct {
	client  *ptpip.Client
	profile Profile
	info    Info
	addr    string
}

// New creates a disconnected Camera.
func New() *Camera {
	return &Camera{client: ptpip.NewClient("aero-shutter"), profile: GenericProfile}
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
	return nil
}

// Disconnect closes the PTP session and both TCP connections.
func (c *Camera) Disconnect() error {
	return c.client.Close()
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

// GetThumb returns the embedded JPEG thumbnail for a handle, or nil when the
// profile disables thumbnails.
func (c *Camera) GetThumb(ctx context.Context, handle uint32) ([]byte, error) {
	if c.profile.Thumbnails == ThumbNone {
		return nil, nil
	}
	return c.client.GetThumb(ctx, handle)
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
