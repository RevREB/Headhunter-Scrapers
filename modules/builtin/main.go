// Command builtin scrapes Built In (builtin.com), a Drupal-backed tech job board
// / aggregator. It needs no HTML-selector parsing: every listing page embeds a
// schema.org ItemList and every detail page a full schema.org JobPosting, both
// as JSON-LD, so this module is stdlib-only. It enumerates job URLs from listing
// pages, then reads each detail page's JobPosting for the full record.
//
// Because Built In is an aggregator (postings often duplicate jobs Headhunter
// already pulls from per-company ATS scrapers), the whole JobPosting graph is
// kept verbatim in Raw — including hiringOrganization.sameAs (the company's
// Built In page) and directApply — so it can later seed new companies/ATSes into
// the library. Titles are pre-filtered against ROLE_KEYWORDS before a detail
// page is fetched, since Built In's volume makes blind detail fetches wasteful.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RevREB/Headhunter-Scrapers/scraperkit"
)

const (
	base         = "https://builtin.com"
	defaultPaths = "jobs" // listing path(s); may be faceted, e.g. jobs/remote/dev-engineering
	// A real browser UA — Built In is fronted by Cloudflare, which serves plain
	// GETs but only with a browser-shaped User-Agent.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

func main() { scraperkit.Main("builtin", fetch) }

func fetch(cfg scraperkit.Config, emit func(scraperkit.RawPosting) bool) error {
	paths := splitCSV(scraperkit.Env("BUILTIN_PATHS", defaultPaths))
	maxPages := atoiDefault(scraperkit.Env("BUILTIN_MAX_PAGES", "3"), 3)
	delay := time.Duration(atoiDefault(scraperkit.Env("BUILTIN_DELAY_MS", "600"), 600)) * time.Millisecond

	for _, path := range paths {
		path = strings.Trim(strings.TrimSpace(path), "/")
		if path == "" {
			continue
		}
		matched, seenDetails := 0, 0
		// page 1 is the first/newest page; page 0 is empty on Built In.
		for page := 1; page <= maxPages; page++ {
			listURL := fmt.Sprintf("%s/%s?page=%d", base, path, page)
			doc, err := getHTML(listURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[builtin] %s page %d: %v\n", path, page, err)
				break
			}
			items := parseListItems(doc)
			if len(items) == 0 {
				break // ran past the last page
			}
			for _, it := range items {
				// Pre-filter on the listing title so we don't spend a detail
				// request on a role we'll drop. scraperkit re-checks in emit.
				if !scraperkit.MatchAny(strings.ToLower(it.Name), cfg.Keywords) {
					continue
				}
				time.Sleep(delay)
				seenDetails++
				jhtml, err := getHTML(it.URL)
				if err != nil {
					fmt.Fprintf(os.Stderr, "[builtin] detail %s: %v\n", it.URL, err)
					continue
				}
				rp, ok := parseJob(jhtml, it.URL, it.Name)
				if !ok {
					fmt.Fprintf(os.Stderr, "[builtin] no JobPosting on %s\n", it.URL)
					continue
				}
				if emit(rp) {
					matched++
				}
			}
			time.Sleep(delay)
		}
		fmt.Fprintf(os.Stderr, "[builtin] %s: %d matched (%d details fetched)\n", path, matched, seenDetails)
	}
	return nil
}

// ---- HTTP ----

func getHTML(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := scraperkit.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return string(b), err
}

// ---- JSON-LD ----

// Built In encodes the '+' in the script type as the HTML entity &#x2B;, so the
// match is loose on ld<+ or entity>json. Case-insensitive covers &#x2b;/&#x2B;.
var ldRe = regexp.MustCompile(`(?is)<script[^>]*type="application/ld(?:&#x2b;|\+)json"[^>]*>(.*?)</script>`)

// ldNodes returns every JSON-LD object in the document, flattening any @graph
// wrapper so ItemList / JobPosting nodes surface at the top level.
func ldNodes(doc string) []map[string]any {
	var out []map[string]any
	for _, m := range ldRe.FindAllStringSubmatch(doc, -1) {
		var v any
		if json.Unmarshal([]byte(strings.TrimSpace(m[1])), &v) != nil {
			continue
		}
		collectNodes(v, &out)
	}
	return out
}

func collectNodes(v any, out *[]map[string]any) {
	switch x := v.(type) {
	case map[string]any:
		if g, ok := x["@graph"]; ok {
			collectNodes(g, out)
			return
		}
		*out = append(*out, x)
	case []any:
		for _, e := range x {
			collectNodes(e, out)
		}
	}
}

// ---- listing ----

type listItem struct{ URL, Name string }

func parseListItems(doc string) []listItem {
	var items []listItem
	for _, n := range ldNodes(doc) {
		if t, _ := n["@type"].(string); t != "ItemList" {
			continue
		}
		els, _ := n["itemListElement"].([]any)
		for _, e := range els {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			u, _ := m["url"].(string)
			nm, _ := m["name"].(string)
			if u != "" {
				items = append(items, listItem{URL: u, Name: nm})
			}
		}
	}
	return items
}

// ---- detail ----

type jobPosting struct {
	Title              string `json:"title"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation                   json.RawMessage `json:"jobLocation"`
	ApplicantLocationRequirements json.RawMessage `json:"applicantLocationRequirements"`
	BaseSalary                    struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
			UnitText string  `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

func parseJob(doc, url, fallbackTitle string) (scraperkit.RawPosting, bool) {
	for _, n := range ldNodes(doc) {
		if t, _ := n["@type"].(string); t != "JobPosting" {
			continue
		}
		raw, _ := json.Marshal(n) // verbatim structured record -> Raw
		var jp jobPosting
		_ = json.Unmarshal(raw, &jp)
		title := jp.Title
		if title == "" {
			title = fallbackTitle
		}
		return scraperkit.RawPosting{
			URL:      url,
			Title:    title,
			Company:  jp.HiringOrganization.Name,
			Location: jp.location(),
			Comp:     jp.comp(),
			PostedAt: normalizeDate(jp.DatePosted),
			Raw:      raw,
		}, true
	}
	return scraperkit.RawPosting{}, false
}

func (j jobPosting) location() string {
	type place struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Region   string `json:"addressRegion"`
			Country  string `json:"addressCountry"`
		} `json:"address"`
	}
	var places []place
	if len(j.JobLocation) > 0 {
		if j.JobLocation[0] == '[' {
			_ = json.Unmarshal(j.JobLocation, &places)
		} else {
			var p place
			if json.Unmarshal(j.JobLocation, &p) == nil {
				places = []place{p}
			}
		}
	}
	var parts []string
	for _, p := range places {
		a := p.Address
		loc := strings.Trim(strings.TrimSpace(a.Locality+", "+a.Region), ", ")
		if loc == "" {
			loc = a.Country
		}
		if loc != "" {
			parts = append(parts, loc)
		}
	}
	if len(parts) == 0 {
		// Remote-only postings usually carry applicantLocationRequirements
		// instead of a physical jobLocation.
		if name := firstName(j.ApplicantLocationRequirements); name != "" {
			return "Remote (" + name + ")"
		}
		return ""
	}
	return strings.Join(dedupeStrings(parts), " · ")
}

func (j jobPosting) comp() string {
	v := j.BaseSalary.Value
	if v.MinValue == 0 && v.MaxValue == 0 {
		return ""
	}
	unit := map[string]string{"YEAR": "yr", "HOUR": "hr", "MONTH": "mo", "WEEK": "wk", "DAY": "day"}[strings.ToUpper(v.UnitText)]
	if unit == "" {
		unit = strings.ToLower(v.UnitText)
	}
	cur := j.BaseSalary.Currency
	if cur == "" {
		cur = "USD"
	}
	switch {
	case v.MinValue > 0 && v.MaxValue > 0 && v.MaxValue != v.MinValue:
		return fmt.Sprintf("%s %s–%s/%s", cur, money(v.MinValue), money(v.MaxValue), unit)
	case v.MaxValue > 0:
		return fmt.Sprintf("%s %s/%s", cur, money(v.MaxValue), unit)
	default:
		return fmt.Sprintf("%s %s/%s", cur, money(v.MinValue), unit)
	}
}

// ---- helpers ----

// firstName pulls a "name" out of a value that may be an object or an array of
// objects (schema.org fields like applicantLocationRequirements vary).
func firstName(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '[' {
		var arr []struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
			return arr[0].Name
		}
		return ""
	}
	var o struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &o)
	return o.Name
}

func money(f float64) string {
	s := strconv.FormatInt(int64(f), 10)
	neg := ""
	if strings.HasPrefix(s, "-") {
		neg, s = "-", s[1:]
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return neg + "$" + string(out)
}

func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 10 { // YYYY-MM-DD -> RFC3339
		return s + "T00:00:00Z"
	}
	return s
}

func splitCSV(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func atoiDefault(s string, d int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
		return n
	}
	return d
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
