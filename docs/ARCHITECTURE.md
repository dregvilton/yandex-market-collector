# Architecture

`cmd/collector` owns startup, signals, CLI flags, and shutdown. `config` loads validated YAML and environment overrides. `collector` starts one worker per browser tab and polls PostgreSQL for due targets. A target claim has a lease so two workers cannot run it simultaneously. `browser` owns the Firefox Nightly processes, persistent contexts, tabs, response handlers, scrolling, parser, and challenge classification. `storage` owns migrations, target/run persistence, observation deduplication, and heartbeats. `export` streams rows. `health` aggregates operational status.

A worker has one stable tab index. A process has exactly the configured number of tabs and its own profile. A normal run navigates, consumes bounded response events, scrolls periodically, stops at source exhaustion or configured limits, and writes batches within transactions. Workers stop on context cancellation. SIGINT/SIGTERM waits for them to finish and writes a final heartbeat.

A source challenge or HTTP 403/429 ends the run, delays its target, and opens a shared in-memory circuit. The circuit reduces requests from other workers at their next scheduling boundary. It is never a CAPTCHA bypass.
