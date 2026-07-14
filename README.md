# Ivory

A small crawler framework in Go. Write a crawler, register it, and manage it from a terminal ui.

## Features

- Terminal ui with a crawler view and a live proxy view, scrolling tables and a `?` help overlay (`-plain` for a simple line shell)
- Proxy pool with rotation strategies, health tracking and cooldown of dead proxies
- Rotating HTTP fetcher (user agent rotation, retries, per host rate limiting, a global concurrency limit)
- Concurrent crawlers, each in its own goroutine, and a panic in one won't take down the rest
- Pluggable storage backends
- JSON configuration

## Running

```
go build -o ivorybin ./cmd/ivory
./ivorybin
```

Use the arrow keys to pick a crawler and `s` / `x` / `r` to start, stop and reload it. `enter` opens a detail view (its config, stats and own logs), `/` filters the list, `e` opens `config.json` in `$EDITOR` and reloads it on save, `tab` switches to the proxy view, `a` starts all, `X` stops all, `?` shows help and `q` quits. Run with `-plain` to get a line based shell instead.

A crawler exposes its own settings to the detail view. There are a few examples in `crawlers/`: `hackernews` (json api), `quotes` (html scraping with pagination), `rss` (xml feed) and `skeleton`, as a template.

## Querying

Records can be searched or dumped with args:

```
ivory -query "einstein"    # print records containing einstein
ivory -export              # dump records to jsonl
```

Configuration is done through `config.json`:

```json
{
  "log_file": "logs/ivory.log",
  "log_level": "info",
  "store": "sqlite",
  "store_config": "data/stories.db",
  "proxies": [],
  "user_agents": ["Mozilla/5.0 ..."],
  "timeout": 15,
  "retries": 3,
  "rate_limit": 200,
  "max_concurrent": 8,
  "max_body_bytes": 10485760,
  "crawlers": ["hackernews"],
  "workers": { "hackernews": 2 },
  "start_on_load": false
}
```

Logs go to `log_file` as JSON (level, time, crawler, message) so they can be shipped or grepped; `log_level` is `debug`, `info`, `warn` or `error`. `workers` runs a crawler as N goroutines (useful for crawlers that pull from a shared frontier). `max_concurrent` caps in flight requests across every crawler and `max_body_bytes` caps a single response so a huge body can't take the process down.

## Durability

The `Store.Save` takes a key that identifies a record. The `sqlite` store upserts on it (WAL mode, so recrawls update instead of duplicating. concurrent workers don't trip over each other) the `jsonl` store dedups by key within a run. On shutdown the engine cancels the crawlers and waits for them to finish before closing the store, so nothing is written into a closed database.

## Writing a Crawler

Implement the `Crawler` interface and register it in an `init`.

```go
type Crawler interface {
	Name() string
	Run(ctx context.Context, rt *Runtime) error
}
```

The `Runtime` gives the crawler what it needs: `Get` for fetching, `Save(key, record)` for storing, `Log` and `Errorf` for output. `Get` retries on network errors and `5xx`, and backs off on `429`/`503` respecting `Retry-After`. Stop and reload cancel the context, so respect `ctx.Done()` in your loop.

## Writing a Store

Implement the `Store` interface and register it in an `init`, like `stores/jsonl.go`. Specify it in the config. There is also a `sqlite` store, set `"store": "sqlite"` and point `store_config` at a database file.

## Proxies

Proxies can be listed inline in `config.json` or dropped into the folder set by `proxy_dir` (default `proxies/`). Files ending in `.txt` are read one proxy per line (`#` comments allowed), `.json` files are a plain array of strings. All sources are merged and deduped.

```
http://user:pass@host:port
socks5://127.0.0.1:1080
```

Each request picks a proxy by `proxy_strategy` (`round_robin`, `random` or `best` by recent success rate). A proxy that fails `proxy_max_fails` times in a row is put on cooldown for `proxy_cooldown` seconds, and success rate is tracked over a sliding window of the last `proxy_window` requests. Press `tab` in the ui to watch it live.

## Known limitations

This is a single node tool, not a distributed system, a few things are out of scope on purpose:

- No headless daemon yet.
- No metrics or health endpoint. Stats live in the ui.
- No robots.txt handling. Point it at things you're allowed to crawl.
- Crawlers don't persist a cursor, so a restart re-crawls from the start (dedup keeps the store clean tho).
- `jsonl` only dedups within a run and never rotates, `sqlite` is single node. Log files don't rotate either.
- Running multiple processes against the same store isn't coordinated.

### License

0BSD See LICENSE. Just do whatever you want with it.