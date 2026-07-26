-- Base schema. CREATE INDEX statements that reference columns added by
-- later migrations must NOT live here — they'd fail on older databases
-- before the migration has a chance to add the column. Put those in
-- storage.go's migrate() instead.

CREATE TABLE IF NOT EXISTS queries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts DATETIME NOT NULL,
  client_ip TEXT NOT NULL,
  domain TEXT NOT NULL,
  query_type TEXT NOT NULL,
  responded INTEGER NOT NULL DEFAULT 1,
  category TEXT NOT NULL DEFAULT 'other'
);

CREATE INDEX IF NOT EXISTS idx_queries_ts ON queries(ts DESC);
CREATE INDEX IF NOT EXISTS idx_queries_client_ip ON queries(client_ip);
CREATE INDEX IF NOT EXISTS idx_queries_domain ON queries(domain);

CREATE TABLE IF NOT EXISTS devices (
  ip TEXT PRIMARY KEY,
  mac TEXT,
  hostname TEXT,
  custom_name TEXT,
  first_seen DATETIME,
  last_seen DATETIME
);
