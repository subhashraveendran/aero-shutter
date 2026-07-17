package camera

import (
	"context"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
)

// scanConcurrency bounds the number of simultaneous probe dials.
const scanConcurrency = 64

// scanProbeTimeout is the per-host connect timeout during a subnet scan.
const scanProbeTimeout = 400 * time.Millisecond

// identifyTimeout bounds the quick GetDeviceInfo connection used to name a
// discovered camera.
const identifyTimeout = 8 * time.Second

// Discovered describes a PTP/IP responder found during detection. Model and
// Serial are empty when the identification handshake failed (for example
// because another client holds the camera's single connection slot).
type Discovered struct {
	// Addr is the address (host or host:port) the responder answered on.
	Addr string
	// Model is the model string from DeviceInfo, e.g. "NIKON D5300".
	Model string
	// Serial is the camera serial number from DeviceInfo.
	Serial string
}

// Detect tries to find a PTP/IP camera. It probes the candidate addresses
// first (e.g. the profile default IP and the last known address), then scans
// the /24 of every local IPv4 interface. It returns the first responding
// address, or an error when nothing answers.
func Detect(ctx context.Context, candidates ...string) (string, error) {
	cams, err := DetectAll(ctx, candidates...)
	if err != nil {
		return "", err
	}
	return cams[0].Addr, nil
}

// DetectAll finds every PTP/IP camera reachable right now: it probes the
// candidate addresses (config/default IPs), scans the /24 of every local
// IPv4 interface for more responders, and identifies each one with a quick
// GetDeviceInfo connection. Candidates that answered are listed first.
func DetectAll(ctx context.Context, candidates ...string) ([]Discovered, error) {
	var primary []string
	for _, addr := range candidates {
		if addr == "" {
			continue
		}
		if ptpip.Probe(ctx, addr, 1500*time.Millisecond) {
			primary = append(primary, addr)
		}
	}
	addrs := mergeAddrs(primary, ScanSubnets(ctx))
	if len(addrs) == 0 {
		return nil, fmt.Errorf("camera: no PTP/IP camera found (tried %d known addresses and local subnets)", len(candidates))
	}
	out := make([]Discovered, 0, len(addrs))
	for _, addr := range addrs {
		d := Discovered{Addr: addr}
		// With a single responder the caller connects to it immediately and
		// reads DeviceInfo itself; skipping identification avoids an extra
		// connect/close cycle, which some bodies dislike in quick succession.
		if len(addrs) > 1 {
			if info, err := Identify(ctx, addr); err == nil {
				d.Model = info.Model
				d.Serial = info.Serial
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// Identify opens a short-lived PTP/IP connection to addr, reads DeviceInfo
// and disconnects, returning the camera's identity.
func Identify(ctx context.Context, addr string) (Info, error) {
	ictx, cancel := context.WithTimeout(ctx, identifyTimeout)
	defer cancel()
	cl := ptpip.NewClient("aero-shutter")
	if err := cl.Connect(ictx, addr); err != nil {
		return Info{}, err
	}
	defer cl.Close()
	di, err := cl.GetDeviceInfo(ictx)
	if err != nil {
		return Info{}, err
	}
	return Info{
		Manufacturer: di.Manufacturer,
		Model:        di.Model,
		Serial:       di.SerialNumber,
		Version:      di.DeviceVersion,
	}, nil
}

// mergeAddrs concatenates primary and extra, dropping duplicates while
// preserving order. Addresses are compared with the default PTP/IP port
// applied, so "192.168.1.1" and "192.168.1.1:15740" are one camera; the
// first spelling seen wins.
func mergeAddrs(primary, extra []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, addr := range append(append([]string{}, primary...), extra...) {
		key := CanonicalAddr(addr)
		if addr == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, addr)
	}
	return out
}

// CanonicalAddr normalises an address to host:port form using the default
// PTP/IP port, so different spellings of the same camera address compare
// equal.
func CanonicalAddr(addr string) string {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return net.JoinHostPort(addr, fmt.Sprint(ptpip.DefaultPort))
	}
	return addr
}

// ScanSubnets probes TCP port 15740 across the /24 of every local IPv4
// interface with bounded concurrency and short timeouts, returning every
// responder sorted by address.
func ScanSubnets(ctx context.Context) []string {
	hosts := subnetHosts()
	if len(hosts) == 0 {
		return nil
	}

	work := make(chan string)
	var mu sync.Mutex
	var found []string
	var wg sync.WaitGroup

	for i := 0; i < scanConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for host := range work {
				if ctx.Err() != nil {
					return
				}
				if ptpip.Probe(ctx, host, scanProbeTimeout) {
					mu.Lock()
					found = append(found, host)
					mu.Unlock()
				}
			}
		}()
	}

feed:
	for _, h := range hosts {
		select {
		case work <- h:
		case <-ctx.Done():
			break feed
		}
	}
	close(work)
	wg.Wait()

	sort.Strings(found)
	return found
}

// subnetHosts enumerates every host address in the /24 of each local IPv4
// interface, excluding our own addresses.
func subnetHosts() []string {
	seen := map[string]bool{}
	var hosts []string
	for _, ip := range localIPv4s() {
		self := ip.String()
		base := net.IPv4(ip[0], ip[1], ip[2], 0).To4()
		key := base.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		for last := 1; last <= 254; last++ {
			host := fmt.Sprintf("%d.%d.%d.%d", base[0], base[1], base[2], last)
			if host == self {
				continue
			}
			hosts = append(hosts, host)
		}
	}
	return hosts
}
