package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dregvilton/yandex-market-collector/internal/browser"
	"github.com/dregvilton/yandex-market-collector/internal/collector"
	"github.com/dregvilton/yandex-market-collector/internal/config"
	"github.com/dregvilton/yandex-market-collector/internal/export"
	"github.com/dregvilton/yandex-market-collector/internal/health"
	"github.com/dregvilton/yandex-market-collector/internal/logging"
	"github.com/dregvilton/yandex-market-collector/internal/storage"
)

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "collector:", e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: yandex-market-collector <run|migrate|status|targets|export>")
	}
	name := args[0]
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	path := fs.String("config", "config.yaml", "YAML configuration file")
	format := fs.String("format", "csv", "csv or jsonl")
	since := fs.String("since", "", "duration (e.g. 24h) or RFC3339")
	until := fs.String("until", "", "duration or RFC3339")
	target := fs.String("target", "", "target key")
	output := fs.String("output", "-", "output path or - for stdout")
	if e := fs.Parse(args[1:]); e != nil {
		return e
	}
	c, e := config.Load(*path)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, e := storage.Open(ctx, c.Database.URL)
	if e != nil {
		return e
	}
	defer s.Close()
	switch name {
	case "migrate":
		return s.Migrate(ctx)
	case "run":
		if e = s.Migrate(ctx); e != nil {
			return e
		}
		if e = s.SyncTargets(ctx, c.Targets); e != nil {
			return e
		}
		pool, e := browser.Open(browser.Options{Processes: c.Browser.Processes, Tabs: c.Browser.TabsPerProcess, Executable: c.Browser.Executable, ProfileDir: c.Browser.ProfileDir, DriverDir: c.Browser.DriverDir, Headless: c.Browser.Headless, ScrollInterval: c.Browser.ScrollInterval.Duration, QueryTimeout: c.Browser.QueryTimeout.Duration})
		if e != nil {
			return e
		}
		defer pool.Close()
		runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		var output io.Writer = os.Stderr
		if path := os.Getenv("COLLECTOR_LOG_FILE"); path != "" {
			file, err := logging.Open(path, 10<<20)
			if err != nil {
				return fmt.Errorf("open log: %w", err)
			}
			defer file.Close()
			output = file
		}
		r := collector.Runner{Store: s, Browser: pool, Config: c, Log: slog.New(slog.NewJSONHandler(output, nil))}
		return r.Run(runCtx)
	case "status":
		st, e := health.Read(ctx, s)
		if e != nil {
			return e
		}
		return health.Write(os.Stdout, st)
	case "targets":
		rows, e := s.DB.Query(ctx, `SELECT key,enabled,query,url,interval_seconds,due_at FROM collection_targets ORDER BY key`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var key, query, url string
			var enabled bool
			var sec int64
			var due time.Time
			if e = rows.Scan(&key, &enabled, &query, &url, &sec, &due); e != nil {
				return e
			}
			fmt.Printf("%s\tenabled=%t\tquery=%q\turl=%q\tinterval=%s\tdue=%s\n", key, enabled, query, url, time.Duration(sec)*time.Second, due.Format(time.RFC3339))
		}
		return rows.Err()
	case "export":
		a, e := parseTime(*since)
		if e != nil {
			return fmt.Errorf("since: %w", e)
		}
		b, e := parseTime(*until)
		if e != nil {
			return fmt.Errorf("until: %w", e)
		}
		return export.File(context.Background(), s, *output, export.Options{Format: strings.ToLower(*format), Target: *target, Since: a, Until: b})
	default:
		return fmt.Errorf("unknown command %q", name)
	}
}
func parseTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	if d, e := time.ParseDuration(s); e == nil {
		v := time.Now().Add(-d)
		return &v, nil
	}
	v, e := time.Parse(time.RFC3339, s)
	if e != nil {
		return nil, e
	}
	return &v, nil
}
