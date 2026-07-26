package storage

import (
	"database/sql"
	_ "embed"
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
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) InsertQuery(q Query) error {
	_, err := s.db.Exec(
		`INSERT INTO queries (ts, client_ip, domain, query_type, responded) VALUES (?, ?, ?, ?, ?)`,
		q.Timestamp, q.ClientIP, q.Domain, q.Type, boolToInt(q.Responded),
	)
	return err
}

func (s *Store) RecentQueries(limit int) ([]Query, error) {
	rows, err := s.db.Query(
		`SELECT id, ts, client_ip, domain, query_type, responded
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
		if err := rows.Scan(&q.ID, &q.Timestamp, &q.ClientIP, &q.Domain, &q.Type, &responded); err != nil {
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
