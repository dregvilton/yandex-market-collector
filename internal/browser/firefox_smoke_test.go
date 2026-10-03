package browser

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestBrowserTopologySmoke(t *testing.T) {
	exe := os.Getenv("TEST_FIREFOX_EXECUTABLE")
	if exe == "" {
		t.Skip("set TEST_FIREFOX_EXECUTABLE for browser integration")
	}
	pool, e := Open(Options{Processes: 2, Tabs: 3, Executable: exe, DriverDir: os.Getenv("TEST_PLAYWRIGHT_DRIVER_DIR"), Headless: true, ScrollInterval: time.Second, QueryTimeout: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if pool.Workers() != 6 {
		t.Fatalf("workers=%d", pool.Workers())
	}
}

func TestLiveAcquisitionSmoke(t *testing.T) {
	if os.Getenv("TEST_LIVE_YANDEX") == "" {
		t.Skip("set TEST_LIVE_YANDEX for one live acquisition")
	}
	exe := os.Getenv("TEST_FIREFOX_EXECUTABLE")
	if exe == "" {
		t.Fatal("TEST_FIREFOX_EXECUTABLE is required")
	}
	pool, e := Open(Options{Processes: 1, Tabs: 1, Executable: exe, DriverDir: os.Getenv("TEST_PLAYWRIGHT_DRIVER_DIR"), Headless: true, ScrollInterval: time.Second, QueryTimeout: 20 * time.Second})
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	received := 0
	stats, e := pool.Collect(ctx, 0, Target{Query: "AMD Ryzen", MaxPages: 1, MaxResults: 10}, func(items []Product) error { received += len(items); return nil })
	t.Logf("stats=%+v products=%d error=%v", stats, received, e)
	if e != nil {
		t.Fatal(e)
	}
	if received == 0 {
		t.Fatal("no source products observed")
	}
}
