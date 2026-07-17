package camera

import (
	"reflect"
	"testing"
)

func TestParseGatewaysDarwin(t *testing.T) {
	// Output of `route -n get default` on macOS.
	out := `   route to: default
destination: default
       mask: default
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC,PRCLONING,GLOBAL>
`
	got := parseGateways("darwin", out)
	if want := []string{"192.168.1.1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("darwin: got %v, want %v", got, want)
	}
}

func TestParseGatewaysDarwinNoGateway(t *testing.T) {
	out := `   route to: default
destination: default
  interface: lo0
`
	if got := parseGateways("darwin", out); len(got) != 0 {
		t.Errorf("darwin without gateway: got %v, want none", got)
	}
}

func TestParseGatewaysLinuxIPRoute(t *testing.T) {
	// Output of `ip route`.
	out := `default via 192.168.1.1 dev wlan0 proto dhcp metric 600
default via 10.0.0.1 dev eth0 proto static metric 100
192.168.1.0/24 dev wlan0 proto kernel scope link src 192.168.1.42
`
	got := parseGateways("linux", out)
	if want := []string{"192.168.1.1", "10.0.0.1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ip route: got %v, want %v", got, want)
	}
}

func TestParseGatewaysLinuxProcNetRoute(t *testing.T) {
	// /proc/net/route: default route (Destination 00000000) whose Gateway
	// column 0101A8C0 is little-endian for 192.168.1.1.
	out := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"wlan0\t00000000\t0101A8C0\t0003\t0\t0\t600\t00000000\t0\t0\t0\n" +
		"wlan0\t0001A8C0\t00000000\t0001\t0\t0\t600\t00FFFFFF\t0\t0\t0\n"
	got := parseGateways("linux", out)
	if want := []string{"192.168.1.1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("proc/net/route: got %v, want %v", got, want)
	}
}

func TestParseGatewaysWindows(t *testing.T) {
	// Output of `route print 0.0.0.0`.
	out := `===========================================================================
Active Routes:
Network Destination        Netmask          Gateway       Interface  Metric
          0.0.0.0          0.0.0.0      192.168.1.1     192.168.1.50     25
      192.168.1.0    255.255.255.0         On-link      192.168.1.50    281
===========================================================================
`
	got := parseGateways("windows", out)
	if want := []string{"192.168.1.1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("windows: got %v, want %v", got, want)
	}
}

func TestParseGatewaysEmpty(t *testing.T) {
	if got := parseGateways("darwin", "   \n  "); got != nil {
		t.Errorf("empty output: got %v, want nil", got)
	}
}

func TestHexLEtoIP(t *testing.T) {
	cases := map[string]string{
		"0101A8C0": "192.168.1.1",
		"0100007F": "127.0.0.1",
		"00000000": "", // default/zero gateway
		"bad":      "", // wrong length
	}
	for in, want := range cases {
		if got := hexLEtoIP(in); got != want {
			t.Errorf("hexLEtoIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDedupIPs(t *testing.T) {
	got := dedupIPs([]string{"192.168.1.1", "", "192.168.1.1", "10.0.0.1"})
	if want := []string{"192.168.1.1", "10.0.0.1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("dedupIPs = %v, want %v", got, want)
	}
}
