package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTargetsAndTopology(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("database:\n  url: postgres://example\nbrowser:\n  processes: 3\n  tabs_per_process: 4\ntargets:\n  - key: arbitrary\n    enabled: true\n    query: widgets\n    interval: 30m\n  - key: direct\n    enabled: true\n    url: https://market.yandex.ru/search?text=thing\n    interval: 1h\n")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	c, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	if c.Browser.Processes != 3 || c.Browser.TabsPerProcess != 4 || len(c.Targets) != 2 {
		t.Fatalf("unexpected config: %+v", c)
	}
	t.Setenv("YANDEX_BROWSER_PROCESSES", "8")
	c, e = Load(p)
	if e != nil || c.Browser.Processes != 8 {
		t.Fatalf("override: %+v %v", c, e)
	}
}
