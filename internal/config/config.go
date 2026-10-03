package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

type Target struct {
	Key        string   `yaml:"key"`
	Enabled    bool     `yaml:"enabled"`
	Query      string   `yaml:"query"`
	URL        string   `yaml:"url"`
	Interval   Duration `yaml:"interval"`
	MaxPages   int      `yaml:"max_pages"`
	MaxResults int      `yaml:"max_results"`
}

type Config struct {
	Database struct {
		URL string `yaml:"url"`
	} `yaml:"database"`
	Browser struct {
		Processes      int      `yaml:"processes"`
		TabsPerProcess int      `yaml:"tabs_per_process"`
		Executable     string   `yaml:"executable"`
		ProfileDir     string   `yaml:"profile_dir"`
		DriverDir      string   `yaml:"driver_dir"`
		Headless       bool     `yaml:"headless"`
		ScrollInterval Duration `yaml:"scroll_interval"`
		QueryTimeout   Duration `yaml:"query_timeout"`
	} `yaml:"browser"`
	Collector struct {
		PollInterval      Duration `yaml:"poll_interval"`
		ChallengeCooldown Duration `yaml:"challenge_cooldown"`
		RetryCooldown     Duration `yaml:"retry_cooldown"`
	} `yaml:"collector"`
	Targets []Target `yaml:"targets"`
}

func Load(path string) (Config, error) {
	var c Config
	data, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("decode config: %w", err)
	}
	if s := os.Getenv("DATABASE_URL"); s != "" {
		c.Database.URL = s
	}
	for _, item := range []struct {
		key  string
		dest *int
	}{{"YANDEX_BROWSER_PROCESSES", &c.Browser.Processes}, {"YANDEX_TABS_PER_BROWSER", &c.Browser.TabsPerProcess}} {
		if s := os.Getenv(item.key); s != "" {
			n, e := strconv.Atoi(s)
			if e != nil {
				return c, fmt.Errorf("%s: %w", item.key, e)
			}
			*item.dest = n
		}
	}
	if s := os.Getenv("YANDEX_FIREFOX_EXECUTABLE"); s != "" {
		c.Browser.Executable = s
	}
	if s := os.Getenv("YANDEX_FIREFOX_PROFILE_DIR"); s != "" {
		c.Browser.ProfileDir = s
	}
	if c.Browser.Processes == 0 {
		c.Browser.Processes = 1
	}
	if c.Browser.TabsPerProcess == 0 {
		c.Browser.TabsPerProcess = 2
	}
	if c.Browser.ScrollInterval.Duration == 0 {
		c.Browser.ScrollInterval.Duration = 5 * time.Second
	}
	if c.Browser.QueryTimeout.Duration == 0 {
		c.Browser.QueryTimeout.Duration = 2 * time.Minute
	}
	if c.Collector.PollInterval.Duration == 0 {
		c.Collector.PollInterval.Duration = 5 * time.Second
	}
	if c.Collector.ChallengeCooldown.Duration == 0 {
		c.Collector.ChallengeCooldown.Duration = 30 * time.Minute
	}
	if c.Collector.RetryCooldown.Duration == 0 {
		c.Collector.RetryCooldown.Duration = 2 * time.Minute
	}
	if c.Database.URL == "" {
		return c, errors.New("database.url or DATABASE_URL is required")
	}
	if c.Browser.Processes < 1 || c.Browser.Processes > 64 || c.Browser.TabsPerProcess < 1 || c.Browser.TabsPerProcess > 32 {
		return c, errors.New("browser topology outside supported 1..64 processes and 1..32 tabs")
	}
	if c.Browser.ScrollInterval.Duration <= 0 || c.Browser.QueryTimeout.Duration <= 0 || c.Collector.PollInterval.Duration <= 0 || c.Collector.ChallengeCooldown.Duration <= 0 || c.Collector.RetryCooldown.Duration <= 0 {
		return c, errors.New("durations must be positive")
	}
	seen := map[string]bool{}
	for _, t := range c.Targets {
		if t.Key == "" || seen[t.Key] {
			return c, fmt.Errorf("empty or duplicate target key %q", t.Key)
		}
		seen[t.Key] = true
		if (strings.TrimSpace(t.Query) == "") == (strings.TrimSpace(t.URL) == "") {
			return c, fmt.Errorf("target %s: set exactly one of query or url", t.Key)
		}
		if t.URL != "" {
			u, e := url.Parse(t.URL)
			if e != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "market.yandex.ru") {
				return c, fmt.Errorf("target %s: URL must use https://market.yandex.ru", t.Key)
			}
		}
		if t.Interval.Duration < time.Second {
			return c, fmt.Errorf("target %s: interval must be positive", t.Key)
		}
		if t.MaxPages < 0 || t.MaxResults < 0 {
			return c, fmt.Errorf("target %s: limits must be nonnegative", t.Key)
		}
	}
	return c, nil
}
