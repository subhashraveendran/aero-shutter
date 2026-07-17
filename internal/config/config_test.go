package config

import (
	"testing"
	"time"
)

func TestUpsertCameraBySerial(t *testing.T) {
	var cfg Config
	t0 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	cfg.UpsertCamera("NIKON D5300", "192.168.1.1", "111", t0)
	cfg.UpsertCamera("NIKON D750", "192.168.1.9", "222", t0.Add(time.Hour))
	if len(cfg.Cameras) != 2 {
		t.Fatalf("got %d cameras, want 2", len(cfg.Cameras))
	}
	if cfg.Cameras[0].Name != "NIKON D750" {
		t.Errorf("newest camera not first: %+v", cfg.Cameras)
	}

	// Same serial on a new address replaces the old entry.
	t1 := t0.Add(2 * time.Hour)
	cfg.UpsertCamera("NIKON D5300", "192.168.1.5", "111", t1)
	if len(cfg.Cameras) != 2 {
		t.Fatalf("upsert duplicated an entry: %+v", cfg.Cameras)
	}
	if got := cfg.Cameras[0]; got.IP != "192.168.1.5" || !got.LastSeen.Equal(t1) {
		t.Errorf("upserted entry = %+v, want new address and time", got)
	}
}

func TestUpsertCameraByIPWithoutSerial(t *testing.T) {
	var cfg Config
	now := time.Now()
	cfg.UpsertCamera("camera", "192.168.1.1", "", now)
	cfg.UpsertCamera("NIKON D5300", "192.168.1.1", "", now.Add(time.Minute))
	if len(cfg.Cameras) != 1 {
		t.Fatalf("got %d cameras, want 1 (matched by IP)", len(cfg.Cameras))
	}
	if cfg.Cameras[0].Name != "NIKON D5300" {
		t.Errorf("entry not updated: %+v", cfg.Cameras[0])
	}
}

func TestDefaultPreviewMode(t *testing.T) {
	if Default().PreviewMode != "auto" {
		t.Errorf("default preview mode = %q, want auto", Default().PreviewMode)
	}
}
