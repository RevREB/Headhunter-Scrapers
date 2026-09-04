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

## Building images

Each module's build definition is its own `modules/<ats>/Dockerfile` — the
language-specific recipe (a Go module runs `go test` + compiles in the build
stage; another language ships its own toolchain Dockerfile). The top-level
workflow (`.github/workflows/build.yml`) is **language-agnostic**: on every push
it discovers module folders (anything with a `Dockerfile`), figures out which ones
changed — a change under `modules/<ats>/` rebuilds that module; a change to
`scraperkit/` or `.github/` rebuilds all — and builds each via the shared
`./.github/actions/scraper-image` composite action, which just runs
`docker/build-push-action` (buildx layer caching) against that module's Dockerfile.
Nothing in the master assumes Go.

> GitHub Actions can't run a workflow file that lives inside a module folder, and
> a step's `uses:` can't take a `${{ matrix.module }}` expression — so the shared
> composite takes the module name as an input and builds that module's own
> Dockerfile. The per-module recipe still lives in the folder; only the generic
> build-and-push-with-cache plumbing is shared.

`scraperkit` is a shared *library* module, not a scraper, so it has no image. Its
unit tests run in a dedicated `test-scraperkit` CI job (a module's own `go test`
can't reach into a separate Go module), and that job gates the image builds.

Images are tagged **`YYYYMMDDHHMM`** (an immutable build id, shared across a run)
**and `latest`**. Core's operator pulls `:latest` with `imagePullPolicy: Always`,
so a freshly-built scraper rolls out on the next scan cycle only if its digest
changed. Build one locally with `docker build -f modules/<ats>/Dockerfile .` from
the repo root.

## How Core runs them

Core's operator reads a catalog and launches one Kubernetes Job per module
(scale-to-zero), on a schedule and on-demand (`POST /api/cycle`). The operative
catalog — images (`:latest`) + per-module company lists — is the RevNet ConfigMap
`apps/headhunter/scan-catalog.yaml`; `catalog/catalog.yaml` here documents the
library. Core never scrapes itself.

## Adding a module

Add `modules/<ats>/` with a `Dockerfile` that honors the contract — for a Go
module also a `go.mod` (pull in `scraperkit` via a local `replace`, see
[`scraperkit/README.md`](scraperkit/README.md)), a `main.go` with a `fetch` func,
and a `_test.go` for any pure helpers. A module in another language just needs its
own toolchain `Dockerfile`. The workflow auto-discovers the folder by its
`Dockerfile` — no matrix to edit — so you only add the module to the RevNet scan
catalog with its company list.

## License

MIT © 2026 RevREB. See [LICENSE](LICENSE).
