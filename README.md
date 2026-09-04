# Headhunter-Scrapers

The ATS scraper library for [Headhunter](https://github.com/RevREB/Headhunter-Core).
Each scraper's only job is to fetch raw postings from **one** ATS (or one
custom-company careers site); all durable logic — dedup, SimHash, trust scoring,
normalization, persistence, analytics — lives in Headhunter-Core. The volatile
edge is this thin, so an ATS breaking never touches the engine.

## What a scraper is

A one-shot batch program (any language; these are stdlib-only Go, built to a
distroless image). Each scraper:

1. reads its config from the environment:
   - `CORE_INGEST_URL` — where to POST results (Core's `/api/scan/ingest`);
   - `ROLE_KEYWORDS` — comma list; keep only postings whose title matches one;
   - an ATS-specific companies var — `GH_COMPANIES`, `LEVER_COMPANIES`,
     `ASHBY_COMPANIES`, `WD_COMPANIES` (`host|tenant|site|Display` tuples),
     `AMAZON_QUERIES`/`AMAZON_COUNTRY`, …;
2. prints a handshake as the first line of stdout
   (`{"ats":…,"contractVersion":"1.0.0","capabilities":["http-json"]}`);
3. fetches + maps postings to the `RawPosting` shape
   (`{url,title,company,location,comp,postedAt,raw}`), deduping by URL;
4. **POSTs the JSON array to `CORE_INGEST_URL`**; logs `[ats] … matched` to
   stderr; exits non-zero on failure.

Core owns everything after ingest (URL-normalize, SimHash dedup, trust, store as
`inbox`).

## Scrapers

| scraper | kind | source |
|---|---|---|
| greenhouse | standard ATS | `boards-api.greenhouse.io` |
| lever | standard ATS | `api.lever.co` |
| ashby | standard ATS | `api.ashbyhq.com` |
| workday | standard ATS | per-tenant CXS API (POST + pagination) |
| amazon | custom company | `amazon.jobs/search.json` |
| apple | custom company | `jobs.apple.com` — **deferred** (CSRF/bot-gated, HTTP 436) |

## How Core runs them

Core's **operator** reads a catalog and launches one Kubernetes Job per scraper
(scale-to-zero), on a schedule and on-demand (`POST /api/cycle`). The operative
catalog — image tags + per-scraper company lists — lives in the RevNet ConfigMap
`apps/headhunter/scan-catalog.yaml`; `catalog/catalog.yaml` here documents the
library. Core never scrapes itself.

## Tiers

- **Tier-2 (all current scrapers)** — a container image in `tier2/<ats>/`, built
  by the `build.yml` matrix to `ghcr.io/revreb/headhunter-scraper-<ats>`.
- **Tier-1 (planned)** — a declarative manifest (endpoints/JSON-paths/field-map)
  run by a shared generic runner via the same operator, so a simple ATS is "just
  a file." `tier1/greenhouse.yaml` shows the intended format; the runner isn't
  built yet.

## Adding an ATS

Add `tier2/<ats>/` (a Go `main` + `Dockerfile` + a `_test.go`), add `<ats>` to
the `build.yml` matrix, then add it to the RevNet scan catalog with its company
list. `go test ./...` runs the per-scraper unit tests (helpers: keyword match,
URL builders, date parsing).

## License

MIT © 2026 RevREB. See [LICENSE](LICENSE).
