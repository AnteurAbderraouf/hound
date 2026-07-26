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
