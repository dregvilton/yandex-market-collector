package storage

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dregvilton/yandex-market-collector/internal/browser"
	"github.com/dregvilton/yandex-market-collector/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ DB *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	db, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, fmt.Errorf("pool: %w", e)
	}
	if e = db.Ping(ctx); e != nil {
		db.Close()
		return nil, fmt.Errorf("database ping: %w", e)
	}
	return &Store{DB: db}, nil
}
func (s *Store) Close() { s.DB.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	for _, name := range []string{"migrations/001_init.sql", "migrations/002_analytics_views.sql"} {
		b, e := migrations.ReadFile(name)
		if e != nil {
			return e
		}
		if _, e = s.DB.Exec(ctx, string(b)); e != nil {
			return fmt.Errorf("apply %s: %w", name, e)
		}
	}
	return nil
}
func (s *Store) SyncTargets(ctx context.Context, targets []config.Target) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE collection_targets SET enabled=false, updated_at=now() WHERE enabled`); err != nil {
		return err
	}
	for _, t := range targets {
		_, e = tx.Exec(ctx, `INSERT INTO collection_targets(key,query,url,enabled,interval_seconds,max_pages,max_results) VALUES($1,$2,$3,$4,$5,$6,$7)
 ON CONFLICT(key) DO UPDATE SET query=excluded.query,url=excluded.url,enabled=excluded.enabled,interval_seconds=excluded.interval_seconds,max_pages=excluded.max_pages,max_results=excluded.max_results,updated_at=now()`, t.Key, t.Query, t.URL, t.Enabled, int64(t.Interval.Duration.Seconds()), t.MaxPages, t.MaxResults)
		if e != nil {
			return fmt.Errorf("sync target %s: %w", t.Key, e)
		}
	}
	return tx.Commit(ctx)
}

type Target struct {
	ID                   int64
	Key, Query, URL      string
	MaxPages, MaxResults int
	IntervalSeconds      int64
}

func (s *Store) Claim(ctx context.Context, lease time.Duration) (*Target, error) {
	var t Target
	e := s.DB.QueryRow(ctx, `WITH next AS (SELECT id FROM collection_targets WHERE enabled AND due_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY due_at FOR UPDATE SKIP LOCKED LIMIT 1)
 UPDATE collection_targets SET lease_until=now()+$1::interval WHERE id IN (SELECT id FROM next)
 RETURNING id,key,query,url,max_pages,max_results,interval_seconds`, fmt.Sprintf("%f seconds", lease.Seconds())).Scan(&t.ID, &t.Key, &t.Query, &t.URL, &t.MaxPages, &t.MaxResults, &t.IntervalSeconds)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return &t, nil
}
func (s *Store) ReleaseClaim(ctx context.Context, targetID int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE collection_targets SET lease_until=NULL WHERE id=$1`, targetID)
	return err
}

func (s *Store) StartRun(ctx context.Context, targetID int64, workerID string) (int64, error) {
	var id int64
	e := s.DB.QueryRow(ctx, `INSERT INTO collection_runs(target_id,worker_id,status) VALUES($1,$2,'running') RETURNING id`, targetID, workerID).Scan(&id)
	return id, e
}

type RunResult struct {
	Status   string
	Stats    browser.Stats
	Saved    int
	Error    string
	Cooldown time.Duration
}

func (s *Store) FinishRun(ctx context.Context, t Target, runID int64, r RunResult) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `UPDATE collection_runs SET completed_at=now(),status=$2,pages=$3,requests=$4,raw_items=$5,parsed_items=$6,saved_items=$7,challenges=$8,http_403=$9,http_429=$10,error=$11 WHERE id=$1`, runID, r.Status, r.Stats.Pages, r.Stats.Requests, r.Stats.RawItems, r.Stats.ParsedItems, r.Saved, r.Stats.Challenges, r.Stats.HTTP403, r.Stats.HTTP429, r.Error)
	if e != nil {
		return e
	}
	delay := max(r.Cooldown, time.Duration(t.IntervalSeconds)*time.Second)
	_, e = tx.Exec(ctx, `UPDATE collection_targets SET due_at=now()+$2::interval,lease_until=NULL WHERE id=$1`, t.ID, fmt.Sprintf("%f seconds", delay.Seconds()))
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) Save(ctx context.Context, targetID, runID int64, products []browser.Product) (int, error) {
	if len(products) == 0 {
		return 0, nil
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var batch pgx.Batch
	for _, p := range products {
		batch.Queue(`WITH changed AS (
   INSERT INTO observation_heads(target_id,source_product_id,source_offer_id,content_hash,observed_at)
   VALUES($1,$3,$4,$16,now())
   ON CONFLICT(target_id,source_product_id,source_offer_id) DO UPDATE
   SET content_hash=excluded.content_hash,observed_at=excluded.observed_at
   WHERE observation_heads.content_hash IS DISTINCT FROM excluded.content_hash
      OR observation_heads.observed_at < now()-interval '24 hours'
   RETURNING 1
  )
  INSERT INTO observations(target_id,run_id,source_product_id,source_offer_id,source_url,title,price_minor,reference_price_minor,currency,seller_source_id,seller_name,rank,availability,image_url,raw_metadata,content_hash)
  SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16 FROM changed`,
			targetID, runID, p.ProductID, p.OfferID, p.URL, p.Title, p.PriceMinor, p.ReferencePriceMinor, p.Currency, p.SellerID, p.SellerName, nullableRank(p.Rank), p.Availability, p.ImageURL, p.Raw, contentHash(p))
	}
	result := tx.SendBatch(ctx, &batch)
	saved := 0
	for range products {
		tag, e := result.Exec()
		if e != nil {
			_ = result.Close()
			return 0, e
		}
		saved += int(tag.RowsAffected())
	}
	if err = result.Close(); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return saved, nil
}
func contentHash(p browser.Product) string {
	v := struct {
		Title, URL, Currency, SellerID, SellerName, Availability, ImageURL string
		Price, Reference                                                   *int64
		SourceFields                                                       map[string]any
	}{p.Title, p.URL, p.Currency, p.SellerID, p.SellerName, p.Availability, p.ImageURL, p.PriceMinor, p.ReferencePriceMinor, nil}
	var raw map[string]any
	_ = json.Unmarshal(p.Raw, &raw)
	v.SourceFields = map[string]any{}
	for _, key := range []string{"badges", "badge", "delivery", "rating", "reviews", "discount", "stockStatus", "additionalPrices"} {
		if x, ok := raw[key]; ok {
			v.SourceFields[key] = x
		}
	}
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Store) Heartbeat(ctx context.Context, started time.Time, processes, tabs, alive int, circuitUntil time.Time, reason string) error {
	var until any
	if !circuitUntil.IsZero() {
		until = circuitUntil
	}
	_, e := s.DB.Exec(ctx, `INSERT INTO collector_state(id,started_at,heartbeat_at,processes,tabs_per_process,workers_alive,circuit_until,circuit_reason) VALUES(1,$1,now(),$2,$3,$4,$5,$6)
 ON CONFLICT(id) DO UPDATE SET started_at=excluded.started_at,heartbeat_at=excluded.heartbeat_at,processes=excluded.processes,tabs_per_process=excluded.tabs_per_process,workers_alive=excluded.workers_alive,circuit_until=excluded.circuit_until,circuit_reason=excluded.circuit_reason`, started, processes, tabs, alive, until, reason)
	return e
}
func (s *Store) ResetStaleRuns(ctx context.Context) error {
	_, e := s.DB.Exec(ctx, `UPDATE collection_runs SET status='error',completed_at=now(),error='collector restarted' WHERE status='running' AND started_at<now()-interval '1 hour'`)
	return e
}
func ClampError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

func nullableRank(rank int) any {
	if rank == 0 {
		return nil
	}
	return rank
}
