# scraperkit

`scraperkit` is a small **Go** package that implements the boilerplate every
Headhunter scraper repeats, so a Go module only has to write the part that is
actually different per ATS: *fetch the raw postings*.

**It is a convenience, not a requirement.** A scraper module can be written in
**any language** — Python, Rust, TypeScript, or a shell script with `curl` +
`jq` — as long as it respects **the contract** below. `scraperkit` is simply the
Go reference implementation of that contract. Non-Go modules reimplement the
~40 lines it covers in their own language.

---

## The contract (language-agnostic)

A scraper is a **one-shot batch program**, shipped as a container image, that the
Headhunter-Core operator runs as a Kubernetes `Job`. It talks to Core over plain
HTTP + JSON. That's the whole contract — anything that satisfies it is a valid
module.

### 1. Inputs — environment variables

| Env var | Required | Meaning |
|---|---|---|
| `CORE_INGEST_URL` | no | Where to POST postings. Defaults to `http://headhunter-core.headhunter.svc.cluster.local:8080/api/scan/ingest`. |
| `ROLE_KEYWORDS` | no | Comma-separated, case-insensitive title filter (e.g. `infrastructure,platform,sre`). Empty ⇒ match everything. |
| *module-specific* | — | Each module defines its own (e.g. a company list, a board slug). Declared by the catalog, not by the kit. |

### 2. Handshake — first line of stdout

Before doing anything else, print **one line of JSON** announcing yourself, so
Core can negotiate the contract version:

```json
{"ats":"greenhouse","contractVersion":"1.0.0","capabilities":["http-json"]}
```

### 3. Output — POST a JSON array of postings

Collect postings, then `POST` them to `CORE_INGEST_URL` as a JSON array with
`Content-Type: application/json`. Each element is a **RawPosting**:

| Field | JSON key | Required | Notes |
|---|---|---|---|
| URL | `url` | **yes** | Canonical posting URL. Also the dedup key — a posting with an empty URL is dropped. |
| Title | `title` | **yes** | Job title. This is what `ROLE_KEYWORDS` matches against. |
| Company | `company` | **yes** | Employer name. |
| Location | `location` | no | Free-text location. |
| Comp | `comp` | no | Compensation string, if the ATS exposes one. |
| Posted at | `postedAt` | no | When the ATS says it was posted (any string; Core parses). |
| Raw | `raw` | no | Arbitrary JSON blob of the original record, preserved verbatim for Core. |

```json
[
  {"url":"https://boards.greenhouse.io/acme/jobs/123","title":"Staff Infrastructure Engineer","company":"Acme","location":"Remote (US)","raw":{...}}
]
```

### 4. Behavior

- **Filter** by `ROLE_KEYWORDS` (case-insensitive substring on the title) so
  Core's inbox stays focused.
- **Dedup** by `url` within a run.
- **Be resilient:** a failure fetching one source should be logged and skipped,
  not abort the whole batch.
- Core normalizes and dedups again server-side, so a missed client-side dedup is
  not fatal — but the filter keeps the inbox clean.

### 5. Exit code

- `0` — success (including "nothing matched"; just skip the POST).
- non-zero — the ingest POST failed or Core answered `>= 300`.

Send all progress/diagnostic logs to **stderr** (stdout's first line is reserved
for the handshake).

---

## What `scraperkit` gives a Go module

`scraperkit.Main(ats, fetch)` does all of the above — handshake, env parsing,
keyword filter, URL dedup, the ingest POST, logging, and the exit code. A Go
module is reduced to a single `fetch` function that calls `emit` for each
posting it finds:

```go
package main

import "github.com/RevREB/Headhunter-Scrapers/scraperkit"

func fetch(cfg scraperkit.Config, emit func(scraperkit.RawPosting) bool) error {
	var board struct {
		Jobs []struct {
			Title, AbsoluteURL, Location string
		} `json:"jobs"`
	}
	// scraperkit.GetJSON + scraperkit.Client are shared helpers.
	if err := scraperkit.GetJSON("https://boards-api.greenhouse.io/v1/boards/acme/jobs", nil, &board); err != nil {
		return err
	}
	for _, j := range board.Jobs {
		emit(scraperkit.RawPosting{
			URL:      j.AbsoluteURL,
			Title:    j.Title,
			Company:  "Acme",
			Location: j.Location,
		})
	}
	return nil
}

func main() { scraperkit.Main("greenhouse", fetch) }
```

`emit` returns `true` when a posting was accepted (passed the keyword filter and
wasn't a duplicate), so a module can keep its own per-source counts. `cfg.Keywords`
and `cfg.IngestURL` are provided if a module wants to inspect them directly.

Helpers exported for module use: `Client` (a shared 30s `*http.Client`), `Env`,
`GetJSON`, and `MatchAny`.

---

## Writing a module in another language

`scraperkit` is Go-only, so a non-Go module skips it and implements the contract
directly. Here is the entire contract as a POSIX shell script — a complete,
valid Headhunter module:

```sh
#!/bin/sh
set -eu
INGEST="${CORE_INGEST_URL:-http://headhunter-core.headhunter.svc.cluster.local:8080/api/scan/ingest}"

# 1. handshake on stdout
echo '{"ats":"acme","contractVersion":"1.0.0","capabilities":["http-json"]}'

# 2. fetch + shape into RawPosting[] (filtering/dedup omitted for brevity)
postings="$(curl -fsS https://acme.example/api/jobs \
  | jq -c '[.jobs[] | {url:.href, title:.name, company:"Acme", location:.loc}]')"

# 3. POST the array; 4. exit code follows curl
echo "[acme] posting $(echo "$postings" | jq length) jobs" >&2
curl -fsS -X POST "$INGEST" -H 'Content-Type: application/json' -d "$postings"
```

Package it in a `Dockerfile`, add it to the build matrix in
`.github/workflows/build.yml` and to the catalog, and Core runs it exactly like
a Go module. Core never knows or cares what language produced the JSON.

---

## Why there is a `go.mod` at the repo root

There is **one** `go.mod` at the root, making the whole repo a single Go module
(`github.com/RevREB/Headhunter-Scrapers`). That is what lets every Go module
`import ".../scraperkit"` and share one dependency set, one `go build ./...`, and
one `go test ./...`. It is the standard "one module, many `main` packages"
Go-monorepo layout — not a per-directory module sprawl.

It does **not** make the repo Go-only. The Go toolchain only looks at directories
that contain `.go` files, so a module written in another language is just a
sibling directory under `modules/` with its own `Dockerfile`; `go build ./...`
ignores it entirely. Go is the *default* here (every current module is Go and
uses `scraperkit`), but the contract — not the `go.mod` — is what defines a module.
