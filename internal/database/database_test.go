package database

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDedupe(t *testing.T) {
	db := openTestDB(t)

	imported, err := db.IsImported(42, "DSC_0042.NEF", 100)
	if err != nil {
		t.Fatalf("IsImported: %v", err)
	}
	if imported {
		t.Fatal("fresh object reported as imported")
	}

	rec := ImportRecord{
		ObjectHandle: 42,
		Filename:     "DSC_0042.NEF",
		Size:         100,
		CaptureDate:  time.Date(2024, 3, 5, 9, 0, 0, 0, time.UTC),
		SizeCheck:    100,
		DestPath:     "/photos/2024/03-05/DSC_0042.NEF",
	}
	if err := db.Record(rec); err != nil {
		t.Fatalf("Record: %v", err)
	}
	// Recording twice must be a silent no-op.
	if err := db.Record(rec); err != nil {
		t.Fatalf("Record (duplicate): %v", err)
	}

	imported, err = db.IsImported(42, "DSC_0042.NEF", 100)
	if err != nil {
		t.Fatalf("IsImported: %v", err)
	}
	if !imported {
		t.Fatal("recorded object not reported as imported")
	}

	// Same handle with a different size is a different object.
	imported, err = db.IsImported(42, "DSC_0042.NEF", 999)
	if err != nil {
		t.Fatalf("IsImported: %v", err)
	}
	if imported {
		t.Fatal("different size should not be deduped")
	}

	set, err := db.ImportedSet()
	if err != nil {
		t.Fatalf("ImportedSet: %v", err)
	}
	if len(set) != 1 {
		t.Fatalf("ImportedSet len = %d, want 1", len(set))
	}
	if !set[Key(42, "DSC_0042.NEF", 100)] {
		t.Error("key missing from ImportedSet")
	}

	dest, err := db.DestPath(42, "DSC_0042.NEF", 100)
	if err != nil {
		t.Fatalf("DestPath: %v", err)
	}
	if dest != rec.DestPath {
		t.Errorf("DestPath = %q, want %q", dest, rec.DestPath)
	}

	today, err := db.ImportedToday()
	if err != nil {
		t.Fatalf("ImportedToday: %v", err)
	}
	if today != 1 {
		t.Errorf("ImportedToday = %d, want 1", today)
	}
}
