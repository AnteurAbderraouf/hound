package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestMigrationFromPreCategorySchema simulates a database created by
// v0.0.2 / v0.0.3 (queries table without the category column) and verifies
// Open() migrates it cleanly instead of erroring out on the category
// index or the SELECT queries.
func TestMigrationFromPreCategorySchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	// Seed a legacy database: queries table without the category column
	// and no category index. This matches the on-disk shape of any db
	// created before v0.0.4.
	{
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatalf("open legacy db: %v", err)
		}
		_, err = db.Exec(`
			CREATE TABLE queries (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				ts DATETIME NOT NULL,
				client_ip TEXT NOT NULL,
				domain TEXT NOT NULL,
				query_type TEXT NOT NULL,
				responded INTEGER NOT NULL DEFAULT 1
			);
			INSERT INTO queries (ts, client_ip, domain, query_type, responded)
			VALUES (CURRENT_TIMESTAMP, '192.168.1.10', 'legacy.example.com', 'A', 1);
		`)
		if err != nil {
			t.Fatalf("seed legacy schema: %v", err)
		}
		db.Close()
	}

	// Now open with the current storage code. Should migrate cleanly.
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open on legacy db failed (migration bug regression): %v", err)
	}
	defer store.Close()

	// The pre-existing row should be readable and its category defaulted.
	rows, err := store.RecentQueries(10)
	if err != nil {
		t.Fatalf("RecentQueries: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row from legacy db, got %d", len(rows))
	}
	if rows[0].Category != "other" {
		t.Errorf("legacy row category = %q, want %q", rows[0].Category, "other")
	}

	// New inserts should work with an explicit category.
	if err := store.InsertQuery(Query{
		Timestamp: time.Now(),
		ClientIP:  "192.168.1.20",
		Domain:    "youtube.com",
		Type:      "A",
		Responded: true,
		Category:  "streaming",
	}); err != nil {
		t.Fatalf("InsertQuery after migration: %v", err)
	}

	rows, err = store.RecentQueries(10)
	if err != nil {
		t.Fatalf("RecentQueries after insert: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows after insert, got %d", len(rows))
	}
	if rows[0].Category != "streaming" {
		t.Errorf("new row category = %q, want %q", rows[0].Category, "streaming")
	}
}

// TestOpenFreshDb verifies Open on a nonexistent path creates a working
// database and doesn't get confused by the pragma_table_info check.
func TestOpenFreshDb(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.db")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open on fresh path: %v", err)
	}
	defer store.Close()

	if err := store.InsertQuery(Query{
		Timestamp: time.Now(),
		ClientIP:  "10.0.0.1",
		Domain:    "example.com",
		Type:      "A",
		Responded: true,
		Category:  "other",
	}); err != nil {
		t.Fatalf("InsertQuery on fresh db: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected db file at %s: %v", path, err)
	}
}

// TestDeviceLifecycle covers the Upsert / List / Rename flow and the
// "don't overwrite good data with empty" guarantee that keeps partial
// enrichment attempts from wiping a real MAC or hostname.
func TestDeviceLifecycle(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "devices.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	t0 := time.Now().Add(-time.Hour)
	t1 := time.Now()

	// First sighting: only IP, no ARP/hostname available yet.
	if err := store.UpsertDevice("192.168.1.42", "", "", t0); err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}
	// Second sighting: ARP resolved a MAC.
	if err := store.UpsertDevice("192.168.1.42", "aa:bb:cc:dd:ee:ff", "", t0.Add(time.Second)); err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}
	// Third sighting: reverse DNS resolved a hostname.
	if err := store.UpsertDevice("192.168.1.42", "", "iPhone-de-Lea", t1); err != nil {
		t.Fatalf("Upsert 3: %v", err)
	}
	// Fourth sighting: ARP now fails (empty MAC). Existing MAC must NOT
	// be overwritten.
	if err := store.UpsertDevice("192.168.1.42", "", "", t1.Add(time.Second)); err != nil {
		t.Fatalf("Upsert 4: %v", err)
	}

	devs, err := store.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devs) != 1 {
		t.Fatalf("want 1 device, got %d", len(devs))
	}
	d := devs[0]
	if d.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("MAC lost across upserts: got %q", d.MAC)
	}
	if d.Hostname != "iPhone-de-Lea" {
		t.Errorf("Hostname lost across upserts: got %q", d.Hostname)
	}
	if d.CustomName != "" {
		t.Errorf("CustomName should default empty, got %q", d.CustomName)
	}

	if err := store.RenameDevice("192.168.1.42", "iPhone Lea"); err != nil {
		t.Fatalf("RenameDevice: %v", err)
	}
	devs, _ = store.ListDevices()
	if devs[0].CustomName != "iPhone Lea" {
		t.Errorf("CustomName after rename = %q", devs[0].CustomName)
	}
	// Rename must not touch MAC/hostname.
	if devs[0].MAC != "aa:bb:cc:dd:ee:ff" || devs[0].Hostname != "iPhone-de-Lea" {
		t.Error("rename accidentally touched enriched fields")
	}

	// Renaming an unknown device returns sql.ErrNoRows.
	if err := store.RenameDevice("10.0.0.99", "x"); err != sql.ErrNoRows {
		t.Errorf("Rename unknown device: got %v, want sql.ErrNoRows", err)
	}
}
