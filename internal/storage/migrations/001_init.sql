CREATE TABLE IF NOT EXISTS collection_targets (
 id BIGSERIAL PRIMARY KEY,
 key TEXT NOT NULL UNIQUE,
 query TEXT NOT NULL DEFAULT '',
 url TEXT NOT NULL DEFAULT '',
 enabled BOOLEAN NOT NULL,
 interval_seconds BIGINT NOT NULL CHECK (interval_seconds > 0),
 max_pages INTEGER NOT NULL DEFAULT 0,
 max_results INTEGER NOT NULL DEFAULT 0,
 due_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 lease_until TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK ((query <> '') <> (url <> ''))
);
CREATE INDEX IF NOT EXISTS collection_targets_due_idx ON collection_targets (due_at) WHERE enabled;
CREATE TABLE IF NOT EXISTS collection_runs (
 id BIGSERIAL PRIMARY KEY,
 target_id BIGINT NOT NULL REFERENCES collection_targets(id),
 worker_id TEXT NOT NULL,
 started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 completed_at TIMESTAMPTZ,
 status TEXT NOT NULL CHECK (status IN ('running','success','error','challenge','cancelled')),
 pages INTEGER NOT NULL DEFAULT 0,
 requests INTEGER NOT NULL DEFAULT 0,
 raw_items INTEGER NOT NULL DEFAULT 0,
 parsed_items INTEGER NOT NULL DEFAULT 0,
 saved_items INTEGER NOT NULL DEFAULT 0,
 challenges INTEGER NOT NULL DEFAULT 0,
 http_403 INTEGER NOT NULL DEFAULT 0,
 http_429 INTEGER NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS collection_runs_target_started_idx ON collection_runs (target_id, started_at DESC);
CREATE INDEX IF NOT EXISTS collection_runs_started_idx ON collection_runs (started_at DESC);
CREATE TABLE IF NOT EXISTS observations (
 id BIGSERIAL PRIMARY KEY,
 target_id BIGINT NOT NULL REFERENCES collection_targets(id),
 run_id BIGINT NOT NULL REFERENCES collection_runs(id),
 observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 source_product_id TEXT NOT NULL,
 source_offer_id TEXT NOT NULL DEFAULT '',
 source_url TEXT NOT NULL DEFAULT '',
 title TEXT NOT NULL,
 price_minor BIGINT,
 reference_price_minor BIGINT,
 currency TEXT NOT NULL,
 seller_source_id TEXT NOT NULL DEFAULT '',
 seller_name TEXT NOT NULL DEFAULT '',
 rank INTEGER,
 availability TEXT NOT NULL DEFAULT '',
 image_url TEXT NOT NULL DEFAULT '',
 raw_metadata JSONB NOT NULL,
 content_hash TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS observations_target_time_idx ON observations (target_id, observed_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS observations_time_idx ON observations (observed_at, id);
CREATE INDEX IF NOT EXISTS observations_identity_time_idx ON observations (target_id, source_product_id, source_offer_id, observed_at DESC);
CREATE TABLE IF NOT EXISTS observation_heads (
 target_id BIGINT NOT NULL REFERENCES collection_targets(id),
 source_product_id TEXT NOT NULL,
 source_offer_id TEXT NOT NULL,
 content_hash TEXT NOT NULL,
 observed_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY (target_id, source_product_id, source_offer_id)
);
CREATE TABLE IF NOT EXISTS collector_state (
 id INTEGER PRIMARY KEY CHECK (id=1),
 started_at TIMESTAMPTZ NOT NULL,
 heartbeat_at TIMESTAMPTZ NOT NULL,
 processes INTEGER NOT NULL,
 tabs_per_process INTEGER NOT NULL,
 workers_alive INTEGER NOT NULL,
 circuit_until TIMESTAMPTZ,
 circuit_reason TEXT NOT NULL DEFAULT ''
);
