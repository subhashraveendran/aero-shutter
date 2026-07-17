// Command aero-shutter is a fast Wi-Fi photo importer for the Nikon D5300,
// speaking PTP/IP over TCP with a terminal user interface.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/subhashraveendran/aero-shutter/internal/config"
	"github.com/subhashraveendran/aero-shutter/internal/database"
	"github.com/subhashraveendran/aero-shutter/internal/frontend"
)

// version is set via -ldflags "-X main.version=..." at build time.
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("aero-shutter", version)
		return
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "aero-shutter:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		// A malformed config falls back to defaults; only path resolution
		// failures are fatal.
		fmt.Fprintln(os.Stderr, "aero-shutter: warning:", err)
	}

	dbPath, err := config.DatabasePath()
	if err != nil {
		return err
	}
	db, err := database.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	return frontend.Run(cfg, db)
}
