// Command workday scrapes Workday tenants via their CXS jobs API (POST +
// pagination). WD_COMPANIES is a comma list of "host|tenant|site|Display" tuples.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/RevREB/Headhunter-Scrapers/scraperkit"
)

const defaultCompanies = "nvidia.wd5.myworkdayjobs.com|nvidia|NVIDIAExternalCareerSite|NVIDIA,boeing.wd1.myworkdayjobs.com|boeing|external_careers|Boeing"

type wdCompany struct{ Host, Tenant, Site, Display string }

type wdResponse struct {
	Total       int `json:"total"`
	JobPostings []struct {
		Title         string `json:"title"`
		ExternalPath  string `json:"externalPath"`
		LocationsText string `json:"locationsText"`
		PostedOn      string `json:"postedOn"`
	} `json:"jobPostings"`
}

// parseCompanies parses "host|tenant|site|Display" tuples; display defaults to
// tenant; malformed tuples (missing host/tenant/site) are skipped.
func parseCompanies(s string) []wdCompany {
	var out []wdCompany
	for _, tok := range strings.Split(s, ",") {
		if tok = strings.TrimSpace(tok); tok == "" {
			continue
		}
		parts := strings.Split(tok, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			continue
		}
		display := parts[1]
		if len(parts) >= 4 && parts[3] != "" {
			display = parts[3]
		}
		out = append(out, wdCompany{Host: parts[0], Tenant: parts[1], Site: parts[2], Display: display})
	}
	return out
}

func fetchPage(c wdCompany, offset int) (*wdResponse, error) {
	url := fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", c.Host, c.Tenant, c.Site)
	reqBody, _ := json.Marshal(map[string]any{"appliedFacets": map[string]any{}, "limit": 20, "offset": offset, "searchText": ""})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := scraperkit.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var wr wdResponse
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil, err
	}
	return &wr, nil
}

// wdDetail is the per-job detail response; jobPostingInfo carries the full JD.
type wdDetail struct {
	JobPostingInfo struct {
		JobDescription string `json:"jobDescription"`
		Location       string `json:"location"`
		StartDate      string `json:"startDate"`
	} `json:"jobPostingInfo"`
}

// fetchDetail GETs one job's detail (the JD body + real posted date). Returns the
// jobPostingInfo object verbatim as Raw plus the parsed fields.
func fetchDetail(c wdCompany, externalPath string) (json.RawMessage, wdDetail, error) {
	url := fmt.Sprintf("https://%s/wday/cxs/%s/%s%s", c.Host, c.Tenant, c.Site, externalPath)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, wdDetail{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := scraperkit.Client.Do(req)
	if err != nil {
		return nil, wdDetail{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, wdDetail{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var d wdDetail
	_ = json.Unmarshal(body, &d)
	var env struct {
		JobPostingInfo json.RawMessage `json:"jobPostingInfo"`
	}
	_ = json.Unmarshal(body, &env)
	return env.JobPostingInfo, d, nil
}

func main() { scraperkit.Main("workday", fetch) }

func fetch(cfg scraperkit.Config, emit func(scraperkit.RawPosting) bool) error {
	for _, c := range parseCompanies(scraperkit.Env("WD_COMPANIES", defaultCompanies)) {
		matched, total := 0, 0
		var perr error
		for offset := 0; offset < 200; offset += 20 {
			wr, err := fetchPage(c, offset)
			if err != nil {
				perr = err
				break
			}
			if len(wr.JobPostings) == 0 {
				break
			}
			for _, jp := range wr.JobPostings {
				total++
				// Workday's list carries no JD, so fetch each job's detail — but only
				// for keyword-matched titles, to keep the extra requests bounded.
				if !scraperkit.MatchAny(strings.ToLower(jp.Title), cfg.Keywords) {
					continue
				}
				p := scraperkit.RawPosting{URL: "https://" + c.Host + jp.ExternalPath, Title: jp.Title, Company: c.Display, Location: jp.LocationsText}
				if rawDetail, det, err := fetchDetail(c, jp.ExternalPath); err == nil && len(rawDetail) > 0 {
					p.Raw = rawDetail
					p.PostedAt = det.JobPostingInfo.StartDate
					if det.JobPostingInfo.Location != "" {
						p.Location = det.JobPostingInfo.Location
					}
				} else {
					raw, _ := json.Marshal(jp)
					p.Raw = raw
					fmt.Fprintf(os.Stderr, "[workday] %s: detail fetch failed for %q (%v)\n", c.Display, jp.Title, err)
				}
				if emit(p) {
					matched++
				}
			}
			if offset+20 >= wr.Total {
				break
			}
		}
		if perr != nil {
			fmt.Fprintf(os.Stderr, "[workday] %s: skipped (%v)\n", c.Display, perr)
			continue
		}
		fmt.Fprintf(os.Stderr, "[workday] %s: %d/%d matched\n", c.Display, matched, total)
	}
	return nil
}
