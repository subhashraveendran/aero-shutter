package frontend

import (
	"context"
	"os/exec"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
	"github.com/subhashraveendran/aero-shutter/internal/database"
	"github.com/subhashraveendran/aero-shutter/internal/importer"
	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
	"github.com/subhashraveendran/aero-shutter/internal/thumbnail"
)

// discoveredMsg reports the result of camera auto-detection: every reachable
// camera on the local subnets plus the configured addresses.
type discoveredMsg struct {
	cams []camera.Discovered
	err  error
}

// connectedMsg reports the result of a connection attempt.
type connectedMsg struct {
	addr string
	err  error
}

// refreshMsg carries a fresh camera inventory.
type refreshMsg struct {
	files    []camera.File
	imported map[string]bool
	storages []camera.Storage
	battery  int
	today    int
	err      error
}

// thumbMsg carries a fetched thumbnail.
type thumbMsg struct {
	handle uint32
	data   []byte
	err    error
}

// importEventMsg wraps one importer progress event; ok is false once the
// event channel is closed.
type importEventMsg struct {
	ev importer.Event
	ok bool
}

// watchTickMsg fires while watch mode is on.
type watchTickMsg struct{}

// cameraEventMsg carries a decoded camera event pushed from the event-reader
// goroutine (e.g. ObjectAdded for instant auto-import).
type cameraEventMsg struct{ ev ptpip.Event }

// linkLostMsg is delivered when the camera link is observed dead in the
// background (event-socket failure or a transaction on a dead socket).
type linkLostMsg struct{ err error }

// reconnectMsg carries an auto-reconnect status update.
type reconnectMsg struct{ state camera.ReconnectState }

// toastClearMsg expires the current toast.
type toastClearMsg struct{ id int }

// openedMsg reports the result of opening a file with the OS handler.
type openedMsg struct{ err error }

// detectCmd probes candidate addresses and local subnets, returning every
// reachable camera with its model name.
func detectCmd(candidates ...string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cams, err := camera.DetectAll(ctx, candidates...)
		return discoveredMsg{cams: cams, err: err}
	}
}

// connectCmd dials the camera and performs the PTP/IP handshake.
func connectCmd(cam *camera.Camera, addr string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := cam.Connect(ctx, addr); err != nil {
			return connectedMsg{addr: addr, err: err}
		}
		return connectedMsg{addr: addr}
	}
}

// refreshCmd lists files, storage and battery from the camera and the
// imported set from the database.
func refreshCmd(cam *camera.Camera, db *database.DB) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		files, err := cam.ListFiles(ctx)
		if err != nil {
			return refreshMsg{err: err}
		}
		imported, err := db.ImportedSet()
		if err != nil {
			return refreshMsg{err: err}
		}
		today, _ := db.ImportedToday()
		battery, berr := cam.BatteryLevel(ctx)
		if berr != nil {
			battery = -1
		}
		storages, _ := cam.StorageInfo(ctx)
		return refreshMsg{files: files, imported: imported, storages: storages, battery: battery, today: today}
	}
}

// thumbCmd fetches the thumbnail for a handle.
func thumbCmd(fetcher *thumbnail.Fetcher, handle uint32) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		data, err := fetcher.Get(ctx, handle)
		return thumbMsg{handle: handle, data: data, err: err}
	}
}

// waitImportEvent receives the next event from an import run.
func waitImportEvent(ch <-chan importer.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		return importEventMsg{ev: ev, ok: ok}
	}
}

// watchTickCmd schedules the next watch-mode poll.
func watchTickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return watchTickMsg{} })
}

// toastClearCmd expires a toast after a delay.
func toastClearCmd(id int) tea.Cmd {
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg { return toastClearMsg{id: id} })
}

// openFileCmd opens a path with the platform file handler: `open` on macOS,
// `xdg-open` on Linux, `cmd /c start` on Windows.
func openFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", path)
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", "", path)
		default:
			cmd = exec.Command("xdg-open", path)
		}
		return openedMsg{err: cmd.Start()}
	}
}
