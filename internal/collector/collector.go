package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dregvilton/yandex-market-collector/internal/browser"
	"github.com/dregvilton/yandex-market-collector/internal/config"
	"github.com/dregvilton/yandex-market-collector/internal/storage"
)

type Runner struct {
	Store         *storage.Store
	Browser       *browser.Pool
	Config        config.Config
	Log           *slog.Logger
	mu            sync.Mutex
	circuitUntil  time.Time
	circuitReason string
}

func (r *Runner) circuit() (time.Time, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.circuitUntil, r.circuitReason
}
func (r *Runner) openCircuit(reason string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	until := time.Now().Add(d)
	if until.After(r.circuitUntil) {
		r.circuitUntil = until
		r.circuitReason = reason
	}
}
func (r *Runner) Run(ctx context.Context) error {
	if r.Store == nil || r.Browser == nil {
		return errors.New("collector needs store and browser")
	}
	if r.Log == nil {
		r.Log = slog.Default()
	}
	if e := r.Store.ResetStaleRuns(ctx); e != nil {
		return e
	}
	started := time.Now()
	var alive atomic.Int32
	alive.Store(int32(r.Browser.Workers()))
	var wg sync.WaitGroup
	for w := 0; w < r.Browser.Workers(); w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			defer alive.Add(-1)
			defer func() {
				if v := recover(); v != nil {
					r.Log.Error("worker panic", "worker", w, "panic", v)
				}
			}()
			r.worker(ctx, w)
		}(w)
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		until, reason := r.circuit()
		heartbeatCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		e := r.Store.Heartbeat(heartbeatCtx, started, r.Config.Browser.Processes, r.Config.Browser.TabsPerProcess, int(alive.Load()), until, reason)
		cancel()
		if e != nil {
			r.Log.Error("heartbeat", "error", e)
		}
		select {
		case <-ctx.Done():
			wg.Wait()
			finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = r.Store.Heartbeat(finalCtx, started, r.Config.Browser.Processes, r.Config.Browser.TabsPerProcess, 0, until, reason)
			finalCancel()
			return nil
		case <-ticker.C:
		}
	}
}
func (r *Runner) worker(ctx context.Context, id int) {
	workerID := fmt.Sprintf("browser-%d/tab-%d", id/r.Config.Browser.TabsPerProcess+1, id%r.Config.Browser.TabsPerProcess+1)
	for ctx.Err() == nil {
		until, _ := r.circuit()
		if wait := time.Until(until); wait > 0 {
			if !sleep(ctx, min(wait, r.Config.Collector.PollInterval.Duration)) {
				return
			}
			continue
		}
		target, e := r.Store.Claim(ctx, r.Config.Browser.QueryTimeout.Duration+time.Minute)
		if e != nil {
			if ctx.Err() == nil {
				r.Log.Error("claim target", "worker", workerID, "error", e)
			}
			if !sleep(ctx, r.Config.Collector.PollInterval.Duration) {
				return
			}
			continue
		}
		if target == nil {
			if !sleep(ctx, r.Config.Collector.PollInterval.Duration) {
				return
			}
			continue
		}
		runID, e := r.Store.StartRun(ctx, target.ID, workerID)
		if e != nil {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = r.Store.ReleaseClaim(releaseCtx, target.ID)
			cancel()
			r.Log.Error("start run", "worker", workerID, "target", target.Key, "error", e)
			if !sleep(ctx, r.Config.Collector.PollInterval.Duration) {
				return
			}
			continue
		}
		r.Log.Info("collection started", "worker", workerID, "target", target.Key, "run", runID)
		saved := 0
		stats, collectErr := r.Browser.Collect(ctx, id, browser.Target{Query: target.Query, URL: target.URL, MaxPages: target.MaxPages, MaxResults: target.MaxResults}, func(products []browser.Product) error {
			n, e := r.Store.Save(ctx, target.ID, runID, products)
			saved += n
			return e
		})
		result := storage.RunResult{Status: "success", Stats: stats, Saved: saved}
		if collectErr != nil {
			result.Error = storage.ClampError(collectErr)
			result.Status = "error"
			if ctx.Err() != nil {
				result.Status = "cancelled"
			}
			var failure *browser.Failure
			if errors.As(collectErr, &failure) {
				if failure.Kind == "CHALLENGE" || failure.Kind == "SOURCE_LIMIT" {
					result.Status = "challenge"
					result.Cooldown = r.Config.Collector.ChallengeCooldown.Duration
					r.openCircuit(failure.Kind, result.Cooldown)
				} else {
					result.Cooldown = r.Config.Collector.RetryCooldown.Duration
				}
			} else {
				result.Cooldown = r.Config.Collector.RetryCooldown.Duration
			}
		}
		finishCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		e = r.Store.FinishRun(finishCtx, *target, runID, result)
		cancel()
		if e != nil {
			r.Log.Error("finish run", "worker", workerID, "target", target.Key, "run", runID, "error", e)
		}
		var failure *browser.Failure
		if errors.As(collectErr, &failure) && failure.Restart {
			if restartErr := r.Browser.Restart(); restartErr != nil {
				r.Log.Error("restart browser", "error", restartErr)
			}
		}
		r.Log.Info("collection finished", "worker", workerID, "target", target.Key, "run", runID, "status", result.Status, "raw", stats.RawItems, "parsed", stats.ParsedItems, "saved", saved, "error", result.Error)
	}
}
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
