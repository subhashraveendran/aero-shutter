package camera

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
)

// scanConcurrency bounds the number of simultaneous probe dials.
const scanConcurrency = 64

// scanProbeTimeout is the per-host connect timeout during a subnet scan.
const scanProbeTimeout = 400 * time.Millisecond

// Detect tries to find a PTP/IP camera. It probes the candidate addresses
// first (e.g. the profile default IP and the last known address), then scans
// the /24 of every local IPv4 interface. It returns the first responding
// address, or an error when nothing answers.
func Detect(ctx context.Context, candidates ...string) (string, error) {
	for _, addr := range candidates {
		if addr == "" {
			continue
		}
		if ptpip.Probe(ctx, addr, 1500*time.Millisecond) {
			return addr, nil
		}
	}
	if addr, ok := ScanSubnets(ctx); ok {
		return addr, nil
	}
	return "", fmt.Errorf("camera: no PTP/IP camera found (tried %d known addresses and local subnets)", len(candidates))
}

// ScanSubnets probes TCP port 15740 across the /24 of every local IPv4
// interface with bounded concurrency and short timeouts. It returns the
// first responder.
func ScanSubnets(ctx context.Context) (string, bool) {
	hosts := subnetHosts()
	if len(hosts) == 0 {
		return "", false
	}

	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	work := make(chan string)
	found := make(chan string, 1)
	var wg sync.WaitGroup

	for i := 0; i < scanConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for host := range work {
				if scanCtx.Err() != nil {
					return
				}
				if ptpip.Probe(scanCtx, host, scanProbeTimeout) {
					select {
					case found <- host:
						cancel()
					default:
					}
					return
				}
			}
		}()
	}

feed:
	for _, h := range hosts {
		select {
		case work <- h:
		case <-scanCtx.Done():
			break feed
		}
	}
	close(work)
	wg.Wait()

	select {
	case addr := <-found:
		return addr, true
	default:
		return "", false
	}
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
