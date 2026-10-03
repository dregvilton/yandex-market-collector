package browser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
)

type Options struct {
	Processes, Tabs                   int
	Executable, ProfileDir, DriverDir string
	Headless                          bool
	ScrollInterval, QueryTimeout      time.Duration
}
type Target struct {
	Query, URL           string
	MaxPages, MaxResults int
}
type Stats struct{ Pages, Requests, RawItems, ParsedItems, Challenges, HTTP403, HTTP429 int }
type Failure struct {
	Kind    string
	Status  int
	Restart bool
}

func (f *Failure) Error() string { return fmt.Sprintf("browser %s (HTTP %d)", f.Kind, f.Status) }

type event struct {
	products            []Product
	page                PageInfo
	raw                 int
	status              int
	challenge           bool
	backgroundChallenge bool
	backgroundStatus    bool
	err                 error
}
type tab struct {
	page       playwright.Page
	events     chan event
	mu         sync.RWMutex
	generation uint64
	requests   map[playwright.Request]uint64
	overflow   atomic.Bool
}
type process struct {
	pw               *playwright.Playwright
	ctx              playwright.BrowserContext
	tabs             []*tab
	temporaryProfile string
}
type Pool struct {
	processes []*process
	opts      Options
	mu        sync.RWMutex
}

func (p *Pool) Workers() int { return p.opts.Processes * p.opts.Tabs }
func (p *Pool) Close() error { p.mu.Lock(); defer p.mu.Unlock(); return p.closeProcesses() }
func (p *Pool) closeProcesses() error {
	var err error
	for _, proc := range p.processes {
		for _, t := range proc.tabs {
			if e := t.page.Close(); e != nil && err == nil {
				err = e
			}
		}
		if e := proc.ctx.Close(); e != nil && err == nil {
			err = e
		}
		if e := proc.pw.Stop(); e != nil && err == nil {
			err = e
		}
		if proc.temporaryProfile != "" {
			_ = os.RemoveAll(proc.temporaryProfile)
		}
	}
	p.processes = nil
	return err
}
func (p *Pool) Restart() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.closeProcesses()
	for i := 0; i < p.opts.Processes; i++ {
		proc, err := openProcess(p.opts, i)
		if err != nil {
			_ = p.closeProcesses()
			return fmt.Errorf("restart browser process %d: %w", i+1, err)
		}
		p.processes = append(p.processes, proc)
	}
	return nil
}
func DiscoverExecutable() string {
	if runtime.GOOS == "darwin" {
		p := "/Applications/Firefox Nightly.app/Contents/MacOS/firefox"
		if st, e := os.Stat(p); e == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}
func Open(o Options) (*Pool, error) {
	if o.Processes < 1 || o.Tabs < 1 {
		return nil, errors.New("positive browser topology required")
	}
	if o.Executable == "" {
		o.Executable = DiscoverExecutable()
	}
	if o.Executable == "" {
		return nil, errors.New("Firefox Nightly executable is required (browser.executable or YANDEX_FIREFOX_EXECUTABLE)")
	}
	if st, e := os.Stat(o.Executable); e != nil || st.IsDir() {
		return nil, fmt.Errorf("Firefox executable unavailable: %s", o.Executable)
	}
	p := &Pool{opts: o}
	for i := 0; i < o.Processes; i++ {
		proc, e := openProcess(o, i)
		if e != nil {
			_ = p.Close()
			return nil, fmt.Errorf("browser process %d: %w", i+1, e)
		}
		p.processes = append(p.processes, proc)
	}
	return p, nil
}
func openProcess(o Options, index int) (*process, error) {
	profile := o.ProfileDir
	temporary := ""
	if profile == "" {
		v, e := os.MkdirTemp("", "yandex-collector-firefox-")
		if e != nil {
			return nil, e
		}
		profile = v
		temporary = v
	} else {
		profile = filepath.Join(profile, fmt.Sprintf("process-%d", index+1))
		if e := os.MkdirAll(profile, 0700); e != nil {
			return nil, e
		}
	}
	if o.DriverDir != "" && os.Getenv("PLAYWRIGHT_NODEJS_PATH") == "" {
		if _, err := os.Stat(filepath.Join(o.DriverDir, "node")); os.IsNotExist(err) {
			if node, e := exec.LookPath("node"); e == nil {
				_ = os.Setenv("PLAYWRIGHT_NODEJS_PATH", node)
			}
		}
	}
	opts := &playwright.RunOptions{}
	if o.DriverDir != "" {
		opts.DriverDirectory = o.DriverDir
	}
	pw, e := playwright.Run(opts)
	if e != nil {
		if temporary != "" {
			_ = os.RemoveAll(temporary)
		}
		return nil, fmt.Errorf("start Playwright: %w", e)
	}
	launch := playwright.BrowserTypeLaunchPersistentContextOptions{ExecutablePath: playwright.String(o.Executable), Headless: playwright.Bool(o.Headless), Locale: playwright.String("ru-RU"), Viewport: &playwright.Size{Width: 390, Height: 844}, IsMobile: playwright.Bool(true), HasTouch: playwright.Bool(true)}
	ctx, e := pw.Firefox.LaunchPersistentContext(profile, launch)
	if e != nil {
		_ = pw.Stop()
		if temporary != "" {
			_ = os.RemoveAll(temporary)
		}
		return nil, fmt.Errorf("launch Firefox Nightly: %w", e)
	}
	proc := &process{pw: pw, ctx: ctx, temporaryProfile: temporary}
	for i := 0; i < o.Tabs; i++ {
		page, e := ctx.NewPage()
		if e != nil {
			for _, t := range proc.tabs {
				_ = t.page.Close()
			}
			_ = ctx.Close()
			_ = pw.Stop()
			return nil, fmt.Errorf("new tab: %w", e)
		}
		t := &tab{page: page, events: make(chan event, 128), requests: make(map[playwright.Request]uint64)}
		install(t)
		proc.tabs = append(proc.tabs, t)
	}
	return proc, nil
}
func install(t *tab) {
	t.page.OnRequest(func(req playwright.Request) { t.mu.Lock(); t.requests[req] = t.generation; t.mu.Unlock() })
	t.page.OnRequestFailed(func(req playwright.Request) { t.mu.Lock(); delete(t.requests, req); t.mu.Unlock() })
	t.page.OnResponse(func(resp playwright.Response) {
		req := resp.Request()
		t.mu.Lock()
		gen, known := t.requests[req]
		delete(t.requests, req)
		t.mu.Unlock()
		if !known {
			return
		}
		u, e := url.Parse(resp.URL())
		if e != nil || !yandexHost(u.Hostname()) {
			return
		}
		typ := req.ResourceType()
		if typ == "image" || typ == "font" || typ == "stylesheet" || typ == "script" || typ == "media" {
			return
		}
		status := resp.Status()
		if status >= 300 && status < 400 {
			return
		}
		ct := strings.ToLower(resp.Headers()["content-type"])
		if typ != "document" && !strings.Contains(ct, "json") && status != 403 && status != 429 {
			return
		}
		body, e := resp.Body()
		if e != nil {
			if !staleResponse(e) && (typ == "document" || acquisitionEndpoint(resp.URL())) {
				t.publish(gen, event{err: fmt.Errorf("read source response: %w", e)})
			}
			return
		}
		if len(body) > 8<<20 {
			if acquisitionEndpoint(resp.URL()) {
				t.publish(gen, event{err: errors.New("source response exceeds 8 MiB")})
			}
			return
		}
		challenge := hardChallenge(body, resp.URL())
		var parsed ParseResult
		var parseErr error
		if strings.Contains(ct, "json") || len(body) > 0 && (body[0] == '{' || body[0] == '[') {
			parsed, parseErr = ParseResponse(body)
		}
		if parseErr != nil && acquisitionEndpoint(resp.URL()) {
			t.publish(gen, event{err: parseErr})
			return
		}
		acquisition := len(parsed.Products) > 0 || parsed.RawRows > 0 || acquisitionEndpoint(resp.URL())
		if challenge {
			if typ == "document" || acquisition {
				t.publish(gen, event{status: status, challenge: true})
			} else {
				t.publish(gen, event{status: status, backgroundChallenge: true})
			}
			return
		}
		if status == 403 || status == 429 {
			if typ == "document" || acquisition {
				t.publish(gen, event{status: status})
			} else {
				t.publish(gen, event{status: status, backgroundStatus: true})
			}
			return
		}
		t.publish(gen, event{products: parsed.Products, page: parsed.Page, raw: parsed.RawRows, status: status})
	})
}
func (t *tab) begin() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.generation++
	t.overflow.Store(false)
	for {
		select {
		case <-t.events:
		default:
			return
		}
	}
}
func (t *tab) publish(gen uint64, e event) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if gen != t.generation {
		return
	}
	select {
	case t.events <- e:
	default:
		t.overflow.Store(true)
	}
}
func (p *Pool) Collect(ctx context.Context, worker int, target Target, emit func([]Product) error) (Stats, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if worker < 0 || worker >= p.Workers() || len(p.processes) != p.opts.Processes {
		return Stats{}, &Failure{Kind: "BROWSER_DISCONNECT", Restart: true}
	}
	proc := p.processes[worker/p.opts.Tabs]
	t := proc.tabs[worker%p.opts.Tabs]
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}
	t.begin()
	u := target.URL
	if u == "" {
		u = "https://market.yandex.ru/search?text=" + url.QueryEscape(target.Query)
	}
	qctx, cancel := context.WithTimeout(ctx, p.opts.QueryTimeout)
	defer cancel()
	var stats Stats
	resp, err := t.page.Goto(u, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded, Timeout: playwright.Float(30000)})
	if err != nil {
		if disconnect(err) {
			return stats, &Failure{Kind: "BROWSER_DISCONNECT", Restart: true}
		}
		return stats, fmt.Errorf("navigate target: %w", err)
	}
	if resp != nil && (resp.Status() == 403 || resp.Status() == 429) {
		if resp.Status() == 403 {
			stats.HTTP403++
		} else {
			stats.HTTP429++
		}
		return stats, &Failure{Kind: "SOURCE_LIMIT", Status: resp.Status()}
	}
	ticker := time.NewTicker(p.opts.ScrollInterval)
	defer ticker.Stop()
	idle := 0
	seen := map[string]bool{}
	for {
		select {
		case <-qctx.Done():
			if t.overflow.Load() {
				return stats, errors.New("browser event queue overflow")
			}
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			return stats, nil
		case ev := <-t.events:
			stats.Requests++
			if ev.backgroundChallenge {
				stats.Challenges++
				continue
			}
			if ev.backgroundStatus {
				if ev.status == 403 {
					stats.HTTP403++
				} else if ev.status == 429 {
					stats.HTTP429++
				}
				continue
			}
			if ev.err != nil {
				return stats, ev.err
			}
			if ev.challenge {
				stats.Challenges++
				return stats, &Failure{Kind: "CHALLENGE", Status: ev.status}
			}
			if ev.status == 403 || ev.status == 429 {
				if ev.status == 403 {
					stats.HTTP403++
				} else {
					stats.HTTP429++
				}
				return stats, &Failure{Kind: "SOURCE_LIMIT", Status: ev.status}
			}
			stats.RawItems += ev.raw
			stats.Pages = max(stats.Pages, ev.page.Page)
			batch := make([]Product, 0, len(ev.products))
			for _, product := range ev.products {
				key := product.ProductID + "\x00" + product.OfferID + "\x00" + string(product.Raw)
				if !seen[key] {
					seen[key] = true
					batch = append(batch, product)
				}
			}
			if len(batch) > 0 {
				idle = 0
				stats.ParsedItems += len(batch)
				if target.MaxResults > 0 && stats.ParsedItems > target.MaxResults {
					batch = batch[:len(batch)-(stats.ParsedItems-target.MaxResults)]
					stats.ParsedItems = target.MaxResults
				}
				if err := emit(batch); err != nil {
					return stats, err
				}
			}
			if target.MaxResults > 0 && stats.ParsedItems >= target.MaxResults {
				return stats, nil
			}
			if target.MaxPages > 0 && stats.Pages >= target.MaxPages {
				return stats, nil
			}
			if ev.page.HasNext != nil && !*ev.page.HasNext {
				return stats, nil
			}
			if ev.page.PageCount > 0 && ev.page.Page >= ev.page.PageCount {
				return stats, nil
			}
		case <-ticker.C:
			if t.overflow.Load() {
				return stats, errors.New("browser event queue overflow")
			}
			if title, e := t.page.Title(); e == nil && challengeTitle(title) {
				stats.Challenges++
				return stats, &Failure{Kind: "CHALLENGE"}
			}
			_, e := t.page.Evaluate(`() => { const nodes=[...document.querySelectorAll('[data-zone-name="productSnippet"],[data-auto="snippet"],article[data-auto]')]; const last=nodes[nodes.length-1]; if(last) last.scrollIntoView({block:'end'}); else window.scrollTo(0,document.body.scrollHeight); }`)
			if e != nil {
				if disconnect(e) {
					return stats, &Failure{Kind: "BROWSER_DISCONNECT", Restart: true}
				}
				return stats, fmt.Errorf("scroll page: %w", e)
			}
			idle++
			if idle >= 8 {
				return stats, nil
			}
		}
	}
}
func challengeTitle(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "captcha") || strings.Contains(s, "подтвердите") || strings.Contains(s, "are you human")
}

func disconnect(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, marker := range []string{"browser closed", "target closed", "page closed", "context closed", "connection closed", "browser disconnected"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func yandexHost(host string) bool {
	host = strings.ToLower(host)
	return host == "yandex.ru" || strings.HasSuffix(host, ".yandex.ru") || host == "yandex.net" || strings.HasSuffix(host, ".yandex.net")
}

func acquisitionEndpoint(rawURL string) bool {
	s := strings.ToLower(rawURL)
	return strings.Contains(s, "resolvepoorremotesearchapphost") || strings.Contains(s, "/api/resolve/")
}
func staleResponse(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "network.getresponsebody") && (strings.Contains(s, "navigated away") || strings.Contains(s, "is not found"))
}
