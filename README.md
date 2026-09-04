# Headhunter-Scrapers

The ATS scraper library for [Headhunter](https://github.com/RevREB/Headhunter-Core).
Each scraper's only job is to fetch raw postings from **one** ATS; all durable
logic (dedup, SimHash, trust, normalization, persistence, analytics) lives in
Headhunter-Core. Because the volatile edge is this thin, an ATS breaking never
touches the engine.

## The contract (v1)

A scraper is a **one-shot batch program**:

1. read `PORTAL_CONFIG` (JSON `PortalConfig`) from the environment;
2. print a `Handshake` as the **first line** of stdout (so Core can validate
   `contractVersion` before trusting output);
3. print a JSON array of `RawPosting`; exit `0` on success, non-zero on failure.

Canonical Go types: `github.com/RevREB/Headhunter-Core/pkg/scraper`. Scrapers can
be written in any language — a scraper is just a program that prints the right JSON.

## Two tiers

- **Tier 1 — declarative** (`tier1/<ats>.yaml`): endpoints, selectors/JSON-paths,
  pagination and a field map, run by a generic engine in Core. Adding an ATS is a
  file. See `tier1/greenhouse.yaml`.
- **Tier 2 — code** (`tier2/<ats>/`): a container image for messy ATSes
  (JS-rendered, anti-bot, odd auth). May drive the shared browser sidecar. See
  `tier2/example/`.

## The catalog = the registry

`catalog/catalog.yaml` maps `ats → tier → manifest|image → contractVersion`. Git
is the registry: Core reads the catalog to know which scrapers exist; Flux ships
it. Adding an ATS = a commit (+ an image push for Tier-2).

## Testing

Each scraper carries a fixture-based contract test (recorded response → asserted
`RawPosting`s) so a broken scraper never ships. `go test ./...` runs them.

## License

MIT © 2026 RevREB. See [LICENSE](LICENSE).
