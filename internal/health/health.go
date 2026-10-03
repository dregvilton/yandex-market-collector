package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"time"

	"github.com/dregvilton/yandex-market-collector/internal/storage"
)

type Status struct {
	Database               string     `json:"database"`
	State                  string     `json:"state"`
	UptimeSeconds          int64      `json:"uptime_seconds"`
	StartedAt              *time.Time `json:"started_at,omitempty"`
	HeartbeatAt            *time.Time `json:"heartbeat_at,omitempty"`
	HeartbeatFresh         bool       `json:"heartbeat_fresh"`
	Processes              int        `json:"processes"`
	TabsPerProcess         int        `json:"tabs_per_process"`
	WorkersAlive           int        `json:"workers_alive"`
	QueuedTargets          int        `json:"queued_targets"`
	RunningCollections     int        `json:"running_collections"`
	CompletedCollections   int        `json:"completed_collections"`
	LatestSuccess          *time.Time `json:"latest_success,omitempty"`
	CircuitUntil           *time.Time `json:"circuit_until,omitempty"`
	CircuitReason          string     `json:"circuit_reason"`
	CircuitState           string     `json:"circuit_state"`
	RecentSuccessRate      float64    `json:"recent_success_rate"`
	ObservationsLastMinute int        `json:"observations_last_minute"`
	RecentRuns             int        `json:"recent_runs"`
	RecentSuccesses        int        `json:"recent_successes"`
	RawItems               int64      `json:"raw_items"`
	ParsedItems            int64      `json:"parsed_items"`
	Errors                 int        `json:"errors"`
	Challenges             int64      `json:"challenges"`
	HTTP403                int64      `json:"http_403"`
	HTTP429                int64      `json:"http_429"`
}

func Read(ctx context.Context, s *storage.Store) (Status, error) {
	var out Status
	out.Database = "ok"
	if e := s.DB.Ping(ctx); e != nil {
		return out, fmt.Errorf("database: %w", e)
	}
	stateErr := s.DB.QueryRow(ctx, `SELECT started_at,heartbeat_at,heartbeat_fresh,processes,tabs_per_process,workers_alive,queued_targets,running_collections,completed_collections,latest_success,circuit_until,circuit_reason FROM collector_health WHERE id=1`).Scan(&out.StartedAt, &out.HeartbeatAt, &out.HeartbeatFresh, &out.Processes, &out.TabsPerProcess, &out.WorkersAlive, &out.QueuedTargets, &out.RunningCollections, &out.CompletedCollections, &out.LatestSuccess, &out.CircuitUntil, &out.CircuitReason)
	if stateErr != nil && !errors.Is(stateErr, pgx.ErrNoRows) {
		return out, stateErr
	}
	e := s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM observations WHERE observed_at>now()-interval '1 minute'),(SELECT count(*) FROM collection_runs WHERE started_at>now()-interval '1 hour'),(SELECT count(*) FROM collection_runs WHERE started_at>now()-interval '1 hour' AND status='success'),(SELECT coalesce(sum(raw_items),0) FROM collection_runs WHERE started_at>now()-interval '1 hour'),(SELECT coalesce(sum(parsed_items),0) FROM collection_runs WHERE started_at>now()-interval '1 hour'),(SELECT count(*) FROM collection_runs WHERE started_at>now()-interval '1 hour' AND status='error'),(SELECT coalesce(sum(challenges),0) FROM collection_runs WHERE started_at>now()-interval '1 hour'),(SELECT coalesce(sum(http_403),0) FROM collection_runs WHERE started_at>now()-interval '1 hour'),(SELECT coalesce(sum(http_429),0) FROM collection_runs WHERE started_at>now()-interval '1 hour')`).Scan(&out.ObservationsLastMinute, &out.RecentRuns, &out.RecentSuccesses, &out.RawItems, &out.ParsedItems, &out.Errors, &out.Challenges, &out.HTTP403, &out.HTTP429)
	out.CircuitState = "closed"
	if out.CircuitUntil != nil && time.Now().Before(*out.CircuitUntil) {
		out.CircuitState = "open"
	}
	if out.RecentRuns > 0 {
		out.RecentSuccessRate = float64(out.RecentSuccesses) / float64(out.RecentRuns)
	}
	if out.StartedAt != nil {
		if out.HeartbeatFresh && out.WorkersAlive > 0 {
			out.State = "running"
			out.UptimeSeconds = int64(time.Since(*out.StartedAt).Seconds())
		} else {
			out.State = "stopped"
		}
	} else {
		out.State = "not_started"
	}
	return out, e
}
func Write(w io.Writer, s Status) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}
