CREATE TABLE IF NOT EXISTS queries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts DATETIME NOT NULL,
  client_ip TEXT NOT NULL,
  domain TEXT NOT NULL,
  query_type TEXT NOT NULL,
  responded INTEGER NOT NULL DEFAULT 1
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
