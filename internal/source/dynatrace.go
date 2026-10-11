package source

// Dynatrace careers search (no auth). The careers site queries a Coveo index
// through its own proxy, which answers without a key:
//
//	POST https://www.dynatrace.com/api/coveo/search/   {"q":"", "numberOfResults":100, "firstResult":N}
//
// Config:
//
//	- name: Dynatrace
//	  source: dynatrace

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
	Register("dynatrace", func(company string, p params.Map, client *http.Client) (Source, error) {
		return &dynatrace{company: company, client: client}, nil
	})
}

type dynatrace struct {
	company string
	client  *http.Client
}

func (s *dynatrace) Company() string { return s.company }

func (s *dynatrace) Fetch(ctx context.Context) ([]model.Job, error) {
	const pageSize = 100
	var jobs []model.Job
	for first := 0; first < 2000; first += pageSize {
		request, _ := json.Marshal(map[string]any{"q": "", "numberOfResults": pageSize, "firstResult": first})
		var page struct {
			TotalCount int `json:"totalCount"`
			Results    []struct {
				Title    string `json:"title"`
				ClickURI string `json:"clickUri"`
				Raw      struct {
					ID        string   `json:"id"`
					Offices   []string `json:"office_locations"`
					Countries []string `json:"country"`
					Date      int64    `json:"date"` // milliseconds
				} `json:"raw"`
			} `json:"results"`
		}
		if err := fetchJSON(ctx, s.client, http.MethodPost, "https://www.dynatrace.com/api/coveo/search/", request, &page); err != nil {
			return nil, err
		}
		for _, r := range page.Results {
			// "Barcelona, Spain" style: offices first, then countries.
			location := strings.Join(append(append([]string{}, r.Raw.Offices...), r.Raw.Countries...), ", ")
			var posted time.Time
			if r.Raw.Date > 0 {
				posted = time.UnixMilli(r.Raw.Date).UTC()
			}
			jobs = append(jobs, model.Job{
				ID:       "dynatrace/" + r.Raw.ID,
				Company:  s.company,
				Title:    strings.TrimSpace(r.Title),
				Location: location,
				URL:      r.ClickURI,
				PostedAt: posted,
			})
		}
		if len(page.Results) < pageSize || first+pageSize >= page.TotalCount {
			break
		}
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("dynatrace: careers search returned no jobs")
	}
	return jobs, nil
}
