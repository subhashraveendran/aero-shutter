package camera

import (
	"context"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// gatewayCmdTimeout bounds the routing-table command used to discover the
// default gateway.
const gatewayCmdTimeout = 2 * time.Second

// DefaultGateways returns the best-effort list of default-gateway IPv4
// addresses for the active non-loopback interfaces. When a camera hosts its
// own Wi-Fi network it is the gateway/DHCP server for that network, so its
// address is almost always the gateway of the interface the user just joined
// (192.168.1.1 for the D5300). Parsing the OS routing table is best-effort;
// callers should treat the result as candidates to probe, not a guarantee.
func DefaultGateways(ctx context.Context) []string {
	out := runRouteCmd(ctx)
	return dedupIPs(parseGateways(runtime.GOOS, out))
}

// runRouteCmd runs the platform routing-table command and returns its output,
// or "" on any error. Kept separate from parsing so parseGateways stays a pure
// function that tests can feed sample output.
func runRouteCmd(ctx context.Context) string {
	cctx, cancel := context.WithTimeout(ctx, gatewayCmdTimeout)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(cctx, "route", "-n", "get", "default")
	case "windows":
		cmd = exec.CommandContext(cctx, "route", "print", "0.0.0.0")
	default: // linux and other unixes
		if b := readProcNetRoute(); b != "" {
			return b
		}
		cmd = exec.CommandContext(cctx, "ip", "route")
	}
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(b)
}

// parseGateways extracts default-gateway IPv4 addresses from the routing-table
// output produced on the given OS. It recognises the formats emitted by:
//
//	darwin:  `route -n get default`      ("gateway: 192.168.1.1")
//	linux:   `ip route`                  ("default via 192.168.1.1 dev …")
//	linux:   `/proc/net/route`           (hex-encoded columns)
//	windows: `route print 0.0.0.0`       ("0.0.0.0  0.0.0.0  192.168.1.1 …")
//
// It is pure (no I/O) so it can be tested against captured sample strings.
func parseGateways(goos, output string) []string {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	switch goos {
	case "darwin":
		return parseDarwinRoute(output)
	case "windows":
		return parseWindowsRoute(output)
	default:
		if gw := parseProcNetRoute(output); len(gw) > 0 {
			return gw
		}
		return parseIPRoute(output)
	}
}

// parseDarwinRoute reads the "gateway:" line of `route -n get default`.
func parseDarwinRoute(output string) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "gateway:" {
			if ip := net.ParseIP(fields[1]); ip != nil && ip.To4() != nil {
				out = append(out, fields[1])
			}
		}
	}
	return out
}

// parseIPRoute reads the gateway from `ip route` "default via <ip> dev …"
// lines (there may be several with different metrics).
func parseIPRoute(output string) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "default" || fields[1] != "via" {
			continue
		}
		if ip := net.ParseIP(fields[2]); ip != nil && ip.To4() != nil {
			out = append(out, fields[2])
		}
	}
	return out
}

// parseWindowsRoute reads the gateway column of `route print` rows whose
// destination and netmask are both 0.0.0.0 (the default route).
func parseWindowsRoute(output string) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "0.0.0.0" || fields[1] != "0.0.0.0" {
			continue
		}
		if ip := net.ParseIP(fields[2]); ip != nil && ip.To4() != nil {
			out = append(out, fields[2])
		}
	}
	return out
}

// parseProcNetRoute reads the default gateway from the contents of
// /proc/net/route. Each data row is tab/space separated; the default route has
// Destination 00000000 and its Gateway column is a little-endian hex IPv4.
func parseProcNetRoute(output string) []string {
	var out []string
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if i == 0 { // header row
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[1] != "00000000" { // Destination != default
			continue
		}
		if ip := hexLEtoIP(fields[2]); ip != "" {
			out = append(out, ip)
		}
	}
	return out
}

// hexLEtoIP converts a little-endian 8-char hex string (as found in
// /proc/net/route) to dotted-quad form, or "" when it is not a valid non-zero
// address.
func hexLEtoIP(h string) string {
	if len(h) != 8 {
		return ""
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil || v == 0 {
		return ""
	}
	b := byte(v)
	b1 := byte(v >> 8)
	b2 := byte(v >> 16)
	b3 := byte(v >> 24)
	return net.IPv4(b, b1, b2, b3).String()
}

// readProcNetRoute returns the contents of /proc/net/route, or "" when it is
// unavailable (any OS without that file, or a read error).
func readProcNetRoute() string {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	return string(b)
}

// dedupIPs removes duplicate and empty entries, preserving order.
func dedupIPs(ips []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, ip := range ips {
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		out = append(out, ip)
	}
	return out
}
