package history

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS incidents (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  window_start TEXT NOT NULL,
  window_end TEXT NOT NULL,
  baseline_start TEXT NOT NULL,
  baseline_end TEXT NOT NULL,
  symptom TEXT NOT NULL DEFAULT '',
  scope TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  notes TEXT NOT NULL DEFAULT '',
  evidence_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS incidents_created_at ON incidents(created_at);

CREATE TABLE IF NOT EXISTS recording_gaps (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  from_ts TEXT NOT NULL,
  to_ts TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS recording_gaps_from_ts ON recording_gaps(from_ts);
`

const duckDBEdgeSchema = `
CREATE TABLE IF NOT EXISTS edge_metrics_5s (
  bucket_start TIMESTAMP NOT NULL,
  edge_id VARCHAR NOT NULL,
  source VARCHAR NOT NULL,
  target VARCHAR NOT NULL,
  protocol VARCHAR NOT NULL,
  port INTEGER NOT NULL,
  connections UBIGINT NOT NULL,
  events UBIGINT NOT NULL,
  requests UBIGINT NOT NULL,
  responses UBIGINT NOT NULL,
  status_2xx UBIGINT NOT NULL,
  status_3xx UBIGINT NOT NULL,
  status_4xx UBIGINT NOT NULL,
  status_5xx UBIGINT NOT NULL,
  status_other UBIGINT NOT NULL,
  p50_ms DOUBLE NOT NULL,
  p95_ms DOUBLE NOT NULL,
  latency_source VARCHAR NOT NULL,
  PRIMARY KEY (bucket_start, edge_id)
);

CREATE TABLE IF NOT EXISTS edge_metrics_1m (
  bucket_start TIMESTAMP NOT NULL,
  edge_id VARCHAR NOT NULL,
  source VARCHAR NOT NULL,
  target VARCHAR NOT NULL,
  protocol VARCHAR NOT NULL,
  port INTEGER NOT NULL,
  connections UBIGINT NOT NULL,
  events UBIGINT NOT NULL,
  requests UBIGINT NOT NULL,
  responses UBIGINT NOT NULL,
  status_2xx UBIGINT NOT NULL,
  status_3xx UBIGINT NOT NULL,
  status_4xx UBIGINT NOT NULL,
  status_5xx UBIGINT NOT NULL,
  status_other UBIGINT NOT NULL,
  p50_ms DOUBLE NOT NULL,
  p95_ms DOUBLE NOT NULL,
  latency_source VARCHAR NOT NULL,
  PRIMARY KEY (bucket_start, edge_id)
);

CREATE TABLE IF NOT EXISTS route_metrics_5s (
  bucket_start TIMESTAMP NOT NULL,
  edge_id VARCHAR NOT NULL,
  method VARCHAR NOT NULL,
  path VARCHAR NOT NULL,
  requests UBIGINT NOT NULL,
  responses UBIGINT NOT NULL,
  status_2xx UBIGINT NOT NULL,
  status_3xx UBIGINT NOT NULL,
  status_4xx UBIGINT NOT NULL,
  status_5xx UBIGINT NOT NULL,
  status_other UBIGINT NOT NULL,
  PRIMARY KEY (bucket_start, edge_id, method, path)
);
`
