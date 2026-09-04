package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const atsName = "workday"

// rawPosting is the contract-shaped output, identical across all scrapers.
type rawPosting struct {
	URL      string          `json:"url"`
	Title    string          `json:"title"`
	Company  string          `json:"company"`
	Location string          `json:"location,omitempty"`
	Comp     string          `json:"comp,omitempty"`
	PostedAt string          `json:"postedAt,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

// wdCompany is a parsed WD_COMPANIES tuple.
type wdCompany struct {
	Host    string
	Tenant  string
	Site    string
	Display string
}

// wdResponse is the shape of the Workday CXS jobs endpoint response.
type wdResponse struct {
	Total       int `json:"total"`
	JobPostings []struct {
		Title         string `json:"title"`
		ExternalPath  string `json:"externalPath"`
		LocationsText string `json:"locationsText"`
		PostedOn      string `json:"postedOn"`
	} `json:"jobPostings"`
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// matchAny reports whether the lowercased title contains any of the keywords.
// An empty keyword slice matches everything (no filtering).
func matchAny(title string, kws []string) bool {
	if len(kws) == 0 {
		return true
	}
	lt := strings.ToLower(title)
	for _, k := range kws {
		if k == "" {
			continue
		}
		if strings.Contains(lt, k) {
			return true
		}
	}
	return false
}

// parseCompanies parses the WD_COMPANIES env value into wdCompany tuples.
// Each tuple is "host|tenant|site|Display"; display is optional and falls
// back to tenant. Malformed tuples (missing host/tenant/site) are skipped.
func parseCompanies(s string) []wdCompany {
	var out []wdCompany
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		parts := strings.Split(tok, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) < 3 {
			continue
		}
		host, tenant, site := parts[0], parts[1], parts[2]
		if host == "" || tenant == "" || site == "" {
			continue
		}
		display := tenant
		if len(parts) >= 4 && parts[3] != "" {
			display = parts[3]
		}
		out = append(out, wdCompany{Host: host, Tenant: tenant, Site: site, Display: display})
	}
	return out
}

// parseKeywords splits a comma list into lowercased, trimmed keywords.
func parseKeywords(s string) []string {
	var out []string
	for _, k := range strings.Split(s, ",") {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

// fetchPage POSTs one page of the Workday CXS jobs endpoint and decodes it.
func fetchPage(httpc *http.Client, c wdCompany, offset int) (*wdResponse, error) {
	url := fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", c.Host, c.Tenant, c.Site)
	reqBody, err := json.Marshal(map[string]any{
		"appliedFacets": map[string]any{},
		"limit":         20,
		"offset":        offset,
		"searchText":    "",
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var wr wdResponse
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil, err
	}
	return &wr, nil
}

// scrapeCompany walks the paginated CXS endpoint for one company and returns
// its matched postings.
func scrapeCompany(httpc *http.Client, c wdCompany, kws []string) ([]rawPosting, int, error) {
	var out []rawPosting
	total := 0
	for offset := 0; offset < 200; offset += 20 {
		wr, err := fetchPage(httpc, c, offset)
		if err != nil {
			return nil, 0, err
		}
		if len(wr.JobPostings) == 0 {
			break
		}
		for _, jp := range wr.JobPostings {
			total++
			if !matchAny(jp.Title, kws) {
				continue
			}
			raw, _ := json.Marshal(jp)
			out = append(out, rawPosting{
				URL:      "https://" + c.Host + jp.ExternalPath,
				Title:    jp.Title,
				Company:  c.Display,
				Location: jp.LocationsText,
				PostedAt: "", // postedOn is relative text, not a real date
				Raw:      json.RawMessage(raw),
			})
		}
		if offset+20 >= wr.Total {
			break
		}
	}
	return out, total, nil
}

func main() {
	// Handshake first.
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"ats":             atsName,
		"contractVersion": "1.0.0",
		"capabilities":    []string{"http-json"},
	})

	ingestURL := env("CORE_INGEST_URL", "http://headhunter-core.headhunter.svc.cluster.local:8080/api/scan/ingest")
	kws := parseKeywords(env("ROLE_KEYWORDS", ""))
	companies := parseCompanies(env("WD_COMPANIES",
		"nvidia.wd5.myworkdayjobs.com|nvidia|NVIDIAExternalCareerSite|NVIDIA,boeing.wd1.myworkdayjobs.com|boeing|external_careers|Boeing"))

	httpc := &http.Client{Timeout: 30 * time.Second}

	var all []rawPosting
	for _, c := range companies {
		posts, total, err := scrapeCompany(httpc, c, kws)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] %s: skipped (%v)\n", atsName, c.Display, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "[%s] %s: %d/%d matched\n", atsName, c.Display, len(posts), total)
		all = append(all, posts...)
	}
	fmt.Fprintf(os.Stderr, "[%s] total matched: %d\n", atsName, len(all))

	if len(all) == 0 {
		return
	}

	payload, err := json.Marshal(all)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] marshal error: %v\n", atsName, err)
		os.Exit(1)
	}
	resp, err := httpc.Post(ingestURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] ingest error: %v\n", atsName, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	fmt.Printf("[%s] ingest -> %d %s\n", atsName, resp.StatusCode, strings.TrimSpace(string(body)))
	if resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
