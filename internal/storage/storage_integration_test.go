package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dregvilton/yandex-market-collector/internal/browser"
	"github.com/dregvilton/yandex-market-collector/internal/config"
	"github.com/dregvilton/yandex-market-collector/internal/export"
	"github.com/dregvilton/yandex-market-collector/internal/health"
	"github.com/dregvilton/yandex-market-collector/internal/storage"
)

func TestPersistenceAndExport(t *testing.T) {
	u := os.Getenv("TEST_DATABASE_URL")
	if u == "" {
		t.Skip("set TEST_DATABASE_URL for PostgreSQL integration")
	}
	ctx := context.Background()
	s, e := storage.Open(ctx, u)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	key := "test-" + time.Now().Format("20060102150405.000000000")
	cfg := config.Target{Key: key, Enabled: true, Query: "widget", Interval: config.Duration{Duration: time.Minute}}
	if e = s.SyncTargets(ctx, []config.Target{cfg}); e != nil {
		t.Fatal(e)
	}
	target, e := s.Claim(ctx, 2*time.Minute)
	if e != nil || target == nil {
		t.Fatalf("claim: %v %v", target, e)
	}
	run, e := s.StartRun(ctx, target.ID, "test-worker")
	if e != nil {
		t.Fatal(e)
	}
	price := int64(123450)
	p := browser.Product{ProductID: "product-1", OfferID: "offer-1", Title: "Widget", PriceMinor: &price, Currency: "RUB", SellerName: "Shop", Raw: json.RawMessage(`{"badge":"new","id":"product-1"}`)}
	n, e := s.Save(ctx, target.ID, run, []browser.Product{p, p})
	if e != nil || n != 1 {
		t.Fatalf("first save: %d %v", n, e)
	}
	n, e = s.Save(ctx, target.ID, run, []browser.Product{p})
	if e != nil || n != 0 {
		t.Fatalf("dedupe: %d %v", n, e)
	}
	changed := p
	newPrice := int64(120000)
	changed.PriceMinor = &newPrice
	n, e = s.Save(ctx, target.ID, run, []browser.Product{changed})
	if e != nil || n != 1 {
		t.Fatalf("changed: %d %v", n, e)
	}
	if e = s.FinishRun(ctx, *target, run, storage.RunResult{Status: "success", Stats: browser.Stats{RawItems: 3, ParsedItems: 3}, Saved: 2}); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM observations WHERE target_id=$1`, target.ID).Scan(&count); e != nil || count != 2 {
		t.Fatalf("history: %d %v", count, e)
	}
	for _, format := range []string{"csv", "jsonl"} {
		var buf bytes.Buffer
		if e = export.Write(ctx, s, &buf, export.Options{Format: format, Target: key}); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(buf.String(), "product-1") || !strings.Contains(buf.String(), "120000") {
			t.Fatalf("%s export: %s", format, buf.String())
		}
	}
	if e = s.Heartbeat(ctx, time.Now(), 2, 3, 6, time.Time{}, ""); e != nil {
		t.Fatal(e)
	}
	status, e := health.Read(ctx, s)
	if e != nil || status.Processes != 2 || status.TabsPerProcess != 3 || !status.HeartbeatFresh {
		t.Fatalf("status: %+v %v", status, e)
	}
}
