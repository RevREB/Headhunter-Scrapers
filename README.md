# Headhunter-Scrapers

The ATS scraper library for [Headhunter](https://github.com/RevREB/Headhunter-Core).
Each **module** fetches raw postings from one ATS (or one custom-company careers
site); all durable logic — dedup, SimHash, trust scoring, normalization,
persistence, analytics — lives in Headhunter-Core. The volatile edge is this
thin, so an ATS breaking never touches the engine.

## Modules

One module per source, each a small Go program in `modules/<ats>/` built to a
distroless image `ghcr.io/revreb/headhunter-scraper-<ats>`:

| module | kind | source |
|---|---|---|
| greenhouse | standard ATS | `boards-api.greenhouse.io` |
| lever | standard ATS | `api.lever.co` |
| ashby | standard ATS | `api.ashbyhq.com` |
| workday | standard ATS | per-tenant CXS API (POST + pagination) |
| amazon | custom company | `amazon.jobs/search.json` |
| apple | custom company | `jobs.apple.com` — **deferred** (CSRF/bot-gated, HTTP 436) |

There are no "tiers" — every source is just a module. (A future *declarative*
module, config instead of code, would be another module backed by a shared
runner; not built yet.)

## scraperkit

`scraperkit` holds everything the modules share: env config, the contract
handshake, keyword filtering, URL dedup, and POSTing to Core's ingest. A module
supplies only a `fetch` function that emits postings:

```go
func main() { scraperkit.Main("greenhouse", fetch) }
func fetch(cfg scraperkit.Config, emit func(scraperkit.RawPosting) bool) error { … }
```

`scraperkit.Main` reads `CORE_INGEST_URL` + `ROLE_KEYWORDS`, prints the handshake,
runs `fetch`, filters by keyword + dedups by URL (that's what `emit` returns),
POSTs the batch, and sets the exit code. A module reads its own companies var
(`GH_COMPANIES`, `ASHBY_COMPANIES`, `WD_COMPANIES` as `host|tenant|site|Display`
tuples, `AMAZON_QUERIES`, …) and calls `emit` per posting.

The full contract — the language-agnostic spec every module must respect, and how
to write a module in **any language** (not just Go) — is in
[`scraperkit/README.md`](scraperkit/README.md).

## How Core runs them

Core's operator reads a catalog and launches one Kubernetes Job per module
(scale-to-zero), on a schedule and on-demand (`POST /api/cycle`). The operative
catalog — image tags + per-module company lists — is the RevNet ConfigMap
`apps/headhunter/scan-catalog.yaml`; `catalog/catalog.yaml` here documents the
library. Core never scrapes itself.

## Adding a module

Add `modules/<ats>/` (a `main.go` with a `fetch` func + `Dockerfile` + a
`_test.go` for any pure helpers), add `<ats>` to the `build.yml` matrix, and add
it to the RevNet scan catalog with its company list. `go test ./...` runs the
unit tests.

## License

MIT © 2026 RevREB. See [LICENSE](LICENSE).
