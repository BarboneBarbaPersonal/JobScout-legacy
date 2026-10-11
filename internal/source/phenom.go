package source

// Phenom People career sites (no auth). The site's own search widget API
// accepts filters, so we only download the countries we care about:
//
//	POST https://{host}/widgets   {"ddoKey":"refineSearch", "selected_fields":{"country":[...]}, "from":N, "size":100, ...}
//
// Config:
//
//	- name: Allianz Technology
//	  source: phenom
//	  params:
//	    host: careers.allianz.com
//	    path: global/en          # site path before /job/{id}
//	    lang: en_global
//	    site_country: global
//	    countries: Spain         # comma-separated, as the site names them

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"jobwatch/internal/model"
	"jobwatch/internal/params"
)

func init() {
	Register("phenom", func(company string, p params.Map, client *http.Client) (Source, error) {
		host, err := p.Require("host")
		if err != nil {
			return nil, err
		}
		var countries []string
		for _, c := range strings.Split(p.Get("countries"), ",") {
			if c = strings.TrimSpace(c); c != "" {
				countries = append(countries, c)
			}
		}
		return &phenom{
			company: company, host: strings.ToLower(strings.TrimSpace(host)),
			path: strings.Trim(p.GetDefault("path", "global/en"), "/"), lang: p.GetDefault("lang", "en_global"),
			siteCountry: p.GetDefault("site_country", "global"), countries: countries, client: client,
		}, nil
	})
}

type phenom struct {
	company, host, path, lang, siteCountry string
	countries                              []string
	client                                 *http.Client
}

func (s *phenom) Company() string { return s.company }

func (s *phenom) Fetch(ctx context.Context) ([]model.Job, error) {
	const pageSize = 100
	selected := map[string][]string{}
	if len(s.countries) > 0 {
		selected["country"] = s.countries
	}
	var jobs []model.Job
	for from := 0; from < 3000; from += pageSize {
		request, _ := json.Marshal(map[string]any{
			"lang": s.lang, "deviceType": "desktop", "country": s.siteCountry, "pageName": "search-results",
			"ddoKey": "refineSearch", "sortBy": "", "subsearch": "", "from": from, "jobs": true, "counts": false,
			"all_fields": []string{"country", "city"}, "size": pageSize, "clearAll": false, "jdsource": "facets",
			"isSliderEnable": false, "keywords": "", "global": true, "selected_fields": selected, "locationData": map[string]any{},
		})
		var page struct {
			RefineSearch struct {
				TotalHits int `json:"totalHits"`
				Data      struct {
					Jobs []struct {
						JobID      string `json:"jobId"`
						Title      string `json:"title"`
						Location   string `json:"location"`
						City       string `json:"city"`
						Country    string `json:"country"`
						PostedDate string `json:"postedDate"`
					} `json:"jobs"`
				} `json:"data"`
			} `json:"refineSearch"`
		}
		if err := fetchJSON(ctx, s.client, http.MethodPost, fmt.Sprintf("https://%s/widgets", s.host), request, &page); err != nil {
			return nil, err
		}
		for _, j := range page.RefineSearch.Data.Jobs {
			location := j.Location
			if location == "" {
				location = strings.Join(nonEmpty(j.City, j.Country), ", ")
			}
			posted, _ := time.Parse("2006-01-02T15:04:05.000-0700", j.PostedDate)
			jobs = append(jobs, model.Job{
				ID:       fmt.Sprintf("phenom/%s/%s", s.host, j.JobID),
				Company:  s.company,
				Title:    strings.TrimSpace(j.Title),
				Location: location,
				URL:      fmt.Sprintf("https://%s/%s/job/%s", s.host, s.path, j.JobID),
				PostedAt: posted,
			})
		}
		if len(page.RefineSearch.Data.Jobs) < pageSize || from+pageSize >= page.RefineSearch.TotalHits {
			break
		}
	}
	return jobs, nil
}
