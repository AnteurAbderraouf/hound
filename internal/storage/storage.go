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

// Device is the persisted view of a machine on the LAN. DisplayName is
// computed by the API layer, not stored.
type Device struct {
	IP         string    `json:"ip"`
	MAC        string    `json:"mac"`
	Vendor     string    `json:"vendor"`
	Hostname   string    `json:"hostname"`
	DeviceType string    `json:"device_type"`
	CustomName string    `json:"custom_name"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
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
func (s *Store) migrate() error {
	// v0.0.4: queries.category
	if err := s.addColumnIfMissing("queries", "category", `TEXT NOT NULL DEFAULT 'other'`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_queries_category ON queries(category)`); err != nil {
		return fmt.Errorf("create idx_queries_category: %w", err)
	}
	// v0.0.8: devices.last_seen index
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_devices_last_seen ON devices(last_seen DESC)`); err != nil {
		return fmt.Errorf("create idx_devices_last_seen: %w", err)
	}
	// v0.0.9: devices.vendor and devices.device_type
	if err := s.addColumnIfMissing("devices", "vendor", `TEXT`); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("devices", "device_type", `TEXT`); err != nil {
		return err
	}
	return nil
}

// addColumnIfMissing performs an ALTER TABLE only when the column is
// absent. Idempotent and safe to call at every startup.
func (s *Store) addColumnIfMissing(table, column, ddl string) error {
	ok, err := s.columnExists(table, column)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if _, err := s.db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, ddl)); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
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

// ---------- queries ---------------------------------------------------------

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

// ---------- devices ---------------------------------------------------------

// UpsertDevice writes device presence at seenAt. Empty mac/hostname/
// vendor values NEVER overwrite an already-populated column so partial
// enrichment attempts can't wipe good data. custom_name and device_type
// have their own dedicated setters.
func (s *Store) UpsertDevice(ip, mac, vendor, hostname string, seenAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO devices (ip, mac, vendor, hostname, first_seen, last_seen)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(ip) DO UPDATE SET
		   last_seen = excluded.last_seen,
		   mac      = CASE WHEN excluded.mac      != '' THEN excluded.mac      ELSE devices.mac      END,
		   vendor   = CASE WHEN excluded.vendor   != '' THEN excluded.vendor   ELSE devices.vendor   END,
		   hostname = CASE WHEN excluded.hostname != '' THEN excluded.hostname ELSE devices.hostname END`,
		ip, mac, vendor, hostname, seenAt, seenAt,
	)
	return err
}

// SetDeviceType records the DNS-fingerprint-derived type for a device.
// Creates a barebones device row if the IP is unknown (this can happen
// when a fingerprint match fires before the ARP-enrichment worker has
// had a chance to insert the row).
func (s *Store) SetDeviceType(ip, deviceType string, seenAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO devices (ip, device_type, first_seen, last_seen)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(ip) DO UPDATE SET
		   device_type = excluded.device_type,
		   last_seen = excluded.last_seen`,
		ip, deviceType, seenAt, seenAt,
	)
	return err
}

// RenameDevice sets custom_name for an existing device. Returns
// sql.ErrNoRows if the device is unknown so the API can 404.
func (s *Store) RenameDevice(ip, customName string) error {
	res, err := s.db.Exec(
		`UPDATE devices SET custom_name = ? WHERE ip = ?`,
		customName, ip,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListDevices returns all devices ordered by most-recently-seen first.
func (s *Store) ListDevices() ([]Device, error) {
	rows, err := s.db.Query(
		`SELECT ip,
		        COALESCE(mac, ''),
		        COALESCE(vendor, ''),
		        COALESCE(hostname, ''),
		        COALESCE(device_type, ''),
		        COALESCE(custom_name, ''),
		        first_seen,
		        last_seen
		 FROM devices
		 ORDER BY last_seen DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Device, 0, 32)
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.IP, &d.MAC, &d.Vendor, &d.Hostname, &d.DeviceType, &d.CustomName, &d.FirstSeen, &d.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
