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
	// ChunkSize is the GetPartialObject chunk size in bytes.
	ChunkSize uint32
}

// D5300Profile is the capability profile for the Nikon D5300. When the D5300
// hosts its own network it assigns itself 192.168.1.1.
var D5300Profile = Profile{
	Name:                  "Nikon D5300",
	ModelMatch:            "D5300",
	DefaultIP:             "192.168.1.1",
	Port:                  15740,
	SupportsPartialObject: true,
	Thumbnails:            ThumbGetThumb,
	ChunkSize:             1 << 20, // 1 MiB
}

// GenericProfile is a conservative fallback used when the reported model does
// not match any known profile.
var GenericProfile = Profile{
	Name:                  "Generic PTP/IP camera",
	ModelMatch:            "",
	DefaultIP:             "192.168.1.1",
	Port:                  15740,
	SupportsPartialObject: false,
	Thumbnails:            ThumbGetThumb,
	ChunkSize:             1 << 20,
}

// Profiles lists all known camera profiles, most specific first.
var Profiles = []Profile{D5300Profile}

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
