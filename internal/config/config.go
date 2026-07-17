// Package config loads and saves the aero-shutter JSON configuration stored in
// the per-user configuration directory.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all persisted settings.
type Config struct {
	// SaveFolder is the root folder photos are imported into.
	SaveFolder string `json:"save_folder"`
	// CameraIP is the address (host or host:port) to connect to.
	CameraIP string `json:"camera_ip"`
	// AutoImport starts importing new files immediately after connecting.
	AutoImport bool `json:"auto_import"`
	// OpenAfterImport opens the destination folder when an import finishes.
	OpenAfterImport bool `json:"open_after_import"`
	// ConcurrentDownloads is reserved for future parallel transfers; the
	// importer currently downloads sequentially regardless of this value.
	ConcurrentDownloads int `json:"concurrent_downloads"`
	// LastConnected is the address of the camera we last spoke to.
	LastConnected string `json:"last_connected"`
}

// Default returns the configuration used when no file exists yet.
func Default() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return Config{
		SaveFolder:          filepath.Join(home, "Pictures", "Nikon"),
		CameraIP:            "192.168.1.1",
		AutoImport:          false,
		OpenAfterImport:     false,
		ConcurrentDownloads: 1,
	}
}

// Dir returns the aero-shutter configuration directory.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: user config dir: %w", err)
	}
	return filepath.Join(base, "aero-shutter"), nil
}

// Path returns the full path of config.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// DatabasePath returns the path of the imports database, colocated with the
// configuration file.
func DatabasePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "imports.db"), nil
}

// Load reads the configuration, filling in defaults for a missing file or
// missing fields.
func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("config: read: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("config: parse %s: %w", path, err)
	}
	if cfg.SaveFolder == "" {
		cfg.SaveFolder = Default().SaveFolder
	}
	if cfg.CameraIP == "" {
		cfg.CameraIP = Default().CameraIP
	}
	if cfg.ConcurrentDownloads < 1 {
		cfg.ConcurrentDownloads = 1
	}
	return cfg, nil
}

// Save writes the configuration atomically (temp file + rename).
func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: create dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return fmt.Errorf("config: temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("config: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: rename: %w", err)
	}
	return nil
}
