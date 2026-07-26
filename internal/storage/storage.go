package storage

import (
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Query struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"ts"`
	ClientIP  string    `json:"client_ip"`
	Domain    string    `json:"domain"`
	Type      string    `json:"type"`
	Responded bool      `json:"responded"`
	Category  string    `json:"category"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply base schema: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// migrate reconciles the on-disk schema with the current version. Each
// step is idempotent so re-running is safe on fresh databases too.
//
// Adding a new column: check pragma_table_info first, then ALTER TABLE.
// Adding an index on a possibly-new column: use CREATE INDEX IF NOT EXISTS
// but only AFTER ensuring the column exists.
func (s *Store) migrate() error {
	// v0.0.4: added queries.category
	if ok, err := s.columnExists("queries", "category"); err != nil {
		return err
	} else if !ok {
		if _, err := s.db.Exec(`ALTER TABLE queries ADD COLUMN category TEXT NOT NULL DEFAULT 'other'`); err != nil {
			return fmt.Errorf("add queries.category: %w", err)
		}
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_queries_category ON queries(category)`); err != nil {
		return fmt.Errorf("create idx_queries_category: %w", err)
	}
	return nil
}

// columnExists reports whether a column exists on a table (uses SQLite's
// pragma_table_info virtual table).
func (s *Store) columnExists(table, column string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`,
		table, column,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) InsertQuery(q Query) error {
	_, err := s.db.Exec(
		`INSERT INTO queries (ts, client_ip, domain, query_type, responded, category)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		q.Timestamp, q.ClientIP, q.Domain, q.Type, boolToInt(q.Responded), q.Category,
	)
	return err
}

func (s *Store) RecentQueries(limit int) ([]Query, error) {
	rows, err := s.db.Query(
		`SELECT id, ts, client_ip, domain, query_type, responded, category
		 FROM queries
		 ORDER BY id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Query, 0, limit)
	for rows.Next() {
		var q Query
		var responded int
		if err := rows.Scan(&q.ID, &q.Timestamp, &q.ClientIP, &q.Domain, &q.Type, &responded, &q.Category); err != nil {
			return nil, err
		}
		q.Responded = responded != 0
		out = append(out, q)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
