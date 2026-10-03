package browser

import (
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
