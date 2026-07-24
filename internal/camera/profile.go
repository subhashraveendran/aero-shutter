// Package camera provides a high-level abstraction over a PTP/IP camera,
// with per-model capability profiles so that supporting a new Nikon body only
// requires adding a profile — the importer and UI stay untouched.
package camera

import "strings"

// ThumbnailStrategy selects how thumbnails are obtained from a model.
type ThumbnailStrategy int

// Thumbnail strategies.
const (
	// ThumbGetThumb uses the standard PTP GetThumb operation, which returns
	// the JPEG thumbnail embedded in the image file.
	ThumbGetThumb ThumbnailStrategy = iota
	// ThumbNone disables thumbnail fetching for models that do not answer
	// GetThumb reliably.
	ThumbNone
)

// Profile describes the capabilities and defaults of a camera model.
type Profile struct {
	// Name is a human-readable profile name.
	Name string
	// ModelMatch is matched (case-insensitively, substring) against the
	// model string reported in DeviceInfo.
	ModelMatch string
	// DefaultIP is the address the camera uses when hosting its own Wi-Fi
	// network.
	DefaultIP string
	// Port is the PTP/IP TCP port.
	Port int
	// SupportsPartialObject reports whether GetPartialObject(0x101B) works,
	// enabling resumable chunked downloads.
	SupportsPartialObject bool
	// Thumbnails selects the thumbnail strategy.
	Thumbnails ThumbnailStrategy
	// LargeThumb enables an attempt at the Nikon vendor operation 0x90C4
	// (GetLargeThumb, a ~640px JPEG). Bodies that reject it fall back to the
	// standard GetThumb automatically for the rest of the session.
	LargeThumb bool
	// ChunkSize is the GetPartialObject chunk size in bytes.
	ChunkSize uint32
}

// nikonWiFiProfile builds the profile shared by Wi-Fi capable Nikon bodies:
// self-assigned 192.168.1.1, PTP/IP on port 15740, resumable chunked
// downloads and large-thumbnail previews with graceful degradation.
func nikonWiFiProfile(name, match string) Profile {
	return Profile{
		Name:                  name,
		ModelMatch:            match,
		DefaultIP:             "192.168.1.1",
		Port:                  15740,
		SupportsPartialObject: true,
		Thumbnails:            ThumbGetThumb,
		LargeThumb:            true,
		ChunkSize:             4 << 20, // 4 MiB: fewer GetPartialObject round-trips = higher Wi-Fi throughput
	}
}

// D5300Profile is the capability profile for the Nikon D5300. When the D5300
// hosts its own network it assigns itself 192.168.1.1.
var D5300Profile = nikonWiFiProfile("Nikon D5300", "D5300")

// GenericProfile is a conservative fallback used when the reported model does
// not match any known profile.
var GenericProfile = Profile{
	Name:                  "Generic PTP/IP camera",
	ModelMatch:            "",
	DefaultIP:             "192.168.1.1",
	Port:                  15740,
	SupportsPartialObject: false,
	Thumbnails:            ThumbGetThumb,
	ChunkSize:             4 << 20,
}

// Profiles lists all known camera profiles, most specific first. Four-digit
// models precede D500 so that substring matching never picks the shorter
// name; Z-series matches use the spaced form ("Z 6") that Nikon reports in
// DeviceInfo.
var Profiles = []Profile{
	nikonWiFiProfile("Nikon D5200", "D5200"),
	D5300Profile,
	nikonWiFiProfile("Nikon D5500", "D5500"),
	nikonWiFiProfile("Nikon D5600", "D5600"),
	nikonWiFiProfile("Nikon D7100", "D7100"),
	nikonWiFiProfile("Nikon D7200", "D7200"),
	nikonWiFiProfile("Nikon D750", "D750"),
	nikonWiFiProfile("Nikon D850", "D850"),
	nikonWiFiProfile("Nikon D500", "D500"),
	nikonWiFiProfile("Nikon Z50", "Z 50"),
	nikonWiFiProfile("Nikon Z6", "Z 6"),
	nikonWiFiProfile("Nikon Z7", "Z 7"),
}

// ProfileForModel returns the profile matching the reported model string,
// falling back to GenericProfile.
func ProfileForModel(model string) Profile {
	m := strings.ToLower(model)
	for _, p := range Profiles {
		if p.ModelMatch != "" && strings.Contains(m, strings.ToLower(p.ModelMatch)) {
			return p
		}
	}
	return GenericProfile
}
