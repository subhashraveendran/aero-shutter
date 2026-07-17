// Package importer implements the download engine: it dedupes against the
// SQLite store, streams files to disk with resume support, and reports
// progress over a channel consumed by the TUI.
package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
	"github.com/subhashraveendran/aero-shutter/internal/database"
	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
)

// Filter selects which camera files an import run considers.
type Filter int

// Import filters.
const (
	FilterAll Filter = iota
	FilterNew
	FilterRAW
	FilterJPEG
	FilterSelected
)

// String returns the display name of the filter.
func (f Filter) String() string {
	switch f {
	case FilterAll:
		return "all"
	case FilterNew:
		return "new"
	case FilterRAW:
		return "raw"
	case FilterJPEG:
		return "jpeg"
	case FilterSelected:
		return "selected"
	default:
		return "unknown"
	}
}

// EventKind discriminates progress events.
type EventKind int

// Progress event kinds.
const (
	EventFileStarted EventKind = iota
	EventFileProgress
	EventFileDone
	EventFileSkipped
	EventAllDone
	EventError
)

// Event is a progress report emitted during an import run.
type Event struct {
	Kind EventKind
	// File is the camera file the event refers to (zero for AllDone).
	File camera.File
	// DestPath is the final on-disk path (FileDone/FileSkipped).
	DestPath string
	// BytesDone is the number of bytes of the current file downloaded.
	BytesDone int64
	// TotalBytesDone counts bytes downloaded across the whole run.
	TotalBytesDone int64
	// TotalBytes is the byte total of the whole run.
	TotalBytes int64
	// Index and Count position the current file within the run.
	Index, Count int
	// Speed is the rolling transfer speed in bytes/second.
	Speed float64
	// ETA estimates remaining time for the whole run.
	ETA time.Duration
	// Err is set for EventError.
	Err error
}

// Options configures an import run.
type Options struct {
	// Filter selects the file subset; FilterSelected uses Selected handles.
	Filter Filter
	// Selected is the handle set for FilterSelected.
	Selected map[uint32]bool
	// SaveFolder overrides the destination root.
	SaveFolder string
}

// Importer downloads files from a camera into the destination tree.
type Importer struct {
	cam *camera.Camera
	db  *database.DB
}

// New creates an Importer bound to a connected camera and open database.
func New(cam *camera.Camera, db *database.DB) *Importer {
	return &Importer{cam: cam, db: db}
}

// DestPath computes the destination path for a file: root/YYYY/MM-DD/name.
// Files with an unknown capture time land in root/unknown-date/name.
func DestPath(root string, f camera.File) string {
	if f.CaptureTime.IsZero() {
		return filepath.Join(root, "unknown-date", f.Name)
	}
	return filepath.Join(
		root,
		f.CaptureTime.Format("2006"),
		f.CaptureTime.Format("01-02"),
		f.Name,
	)
}

// Matches reports whether a file passes the filter given the set of already
// imported keys.
func Matches(f camera.File, opts Options, imported map[string]bool) bool {
	switch opts.Filter {
	case FilterSelected:
		return opts.Selected[f.Handle]
	case FilterNew:
		return !imported[database.Key(f.Handle, f.Name, f.Size)]
	case FilterRAW:
		return f.Format == ptpip.FormatNEF || f.Format == ptpip.FormatUndefined
	case FilterJPEG:
		return f.Format == ptpip.FormatJPEG
	default:
		return true
	}
}

// Run imports the given files according to opts, emitting progress events on
// the returned channel. The channel closes after EventAllDone or a fatal
// EventError. Cancellation via ctx stops after the current chunk; partial
// files are kept as .part for resume.
func (im *Importer) Run(ctx context.Context, files []camera.File, opts Options) <-chan Event {
	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		im.run(ctx, files, opts, ch)
	}()
	return ch
}

func (im *Importer) run(ctx context.Context, files []camera.File, opts Options, ch chan<- Event) {
	imported, err := im.db.ImportedSet()
	if err != nil {
		ch <- Event{Kind: EventError, Err: err}
		return
	}

	var queue []camera.File
	var totalBytes int64
	for _, f := range files {
		if !Matches(f, opts, imported) {
			continue
		}
		queue = append(queue, f)
		totalBytes += f.Size
	}

	speed := newSpeedometer(5 * time.Second)
	var totalDone int64

	for i, f := range queue {
		if err := ctx.Err(); err != nil {
			ch <- Event{Kind: EventError, Err: err}
			return
		}
		key := database.Key(f.Handle, f.Name, f.Size)
		if imported[key] {
			dest, _ := im.db.DestPath(f.Handle, f.Name, f.Size)
			totalDone += f.Size
			ch <- Event{
				Kind: EventFileSkipped, File: f, DestPath: dest,
				Index: i + 1, Count: len(queue),
				TotalBytesDone: totalDone, TotalBytes: totalBytes,
			}
			continue
		}

		dest := DestPath(opts.SaveFolder, f)
		ch <- Event{
			Kind: EventFileStarted, File: f, DestPath: dest,
			Index: i + 1, Count: len(queue),
			TotalBytesDone: totalDone, TotalBytes: totalBytes,
		}

		base := totalDone
		progress := func(written int64) {
			speed.add(written)
			done := base + written
			ch <- Event{
				Kind: EventFileProgress, File: f, DestPath: dest,
				BytesDone: written, Index: i + 1, Count: len(queue),
				TotalBytesDone: done, TotalBytes: totalBytes,
				Speed: speed.rate(), ETA: eta(speed.rate(), totalBytes-done),
			}
		}

		if err := im.downloadOne(ctx, f, dest, progress); err != nil {
			ch <- Event{Kind: EventError, File: f, DestPath: dest, Err: err, Index: i + 1, Count: len(queue)}
			return
		}

		if err := im.db.Record(database.ImportRecord{
			ObjectHandle: f.Handle,
			Filename:     f.Name,
			Size:         f.Size,
			CaptureDate:  f.CaptureTime,
			SizeCheck:    f.Size,
			DestPath:     dest,
		}); err != nil {
			ch <- Event{Kind: EventError, File: f, DestPath: dest, Err: err}
			return
		}
		imported[key] = true
		totalDone += f.Size
		ch <- Event{
			Kind: EventFileDone, File: f, DestPath: dest,
			BytesDone: f.Size, Index: i + 1, Count: len(queue),
			TotalBytesDone: totalDone, TotalBytes: totalBytes,
			Speed: speed.rate(), ETA: eta(speed.rate(), totalBytes-totalDone),
		}
	}

	ch <- Event{Kind: EventAllDone, Count: len(queue), TotalBytesDone: totalDone, TotalBytes: totalBytes, Speed: speed.rate()}
}

// downloadOne streams a single file to dest via a .part file, resuming any
// existing partial download.
func (im *Importer) downloadOne(ctx context.Context, f camera.File, dest string, progress camera.ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("importer: create folder: %w", err)
	}

	part := dest + ".part"
	var offset int64
	if st, err := os.Stat(part); err == nil {
		offset = st.Size()
		if offset > f.Size {
			// Stale partial from a different object; start over.
			offset = 0
		}
	}
	if !im.cam.Profile().SupportsPartialObject {
		offset = 0
	}

	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if offset == 0 {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	out, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return fmt.Errorf("importer: open %s: %w", part, err)
	}

	if err := im.cam.DownloadTo(ctx, out, f, offset, progress); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("importer: close %s: %w", part, err)
	}

	st, err := os.Stat(part)
	if err != nil {
		return fmt.Errorf("importer: stat %s: %w", part, err)
	}
	if st.Size() != f.Size {
		return fmt.Errorf("importer: size mismatch for %s: got %d, want %d", f.Name, st.Size(), f.Size)
	}
	if err := os.Rename(part, dest); err != nil {
		return fmt.Errorf("importer: finalize %s: %w", dest, err)
	}
	if !f.CaptureTime.IsZero() {
		_ = os.Chtimes(dest, f.CaptureTime, f.CaptureTime)
	}
	return nil
}

func eta(rate float64, remaining int64) time.Duration {
	if rate <= 0 || remaining <= 0 {
		return 0
	}
	return time.Duration(float64(remaining) / rate * float64(time.Second)).Round(time.Second)
}
