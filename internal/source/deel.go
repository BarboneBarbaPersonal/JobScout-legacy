package source

// Deel careers page (no auth). Deel hires through a private Ashby board, but
// its Next.js careers page embeds every listed job (escaped JSON inside the
// page's streaming payload) with ashby_id, title, all_locations and dates.
//
//	GET https://www.deel.com/careers/
//
// Config:
//
//	- name: Deel
//	  source: deel

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"jobwatch/internal/model"
	"jobwatch/internal/params"
)

func init() {
	Register("deel", func(company string, p params.Map, client *http.Client) (Source, error) {
		return &deel{company: company, client: client}, nil
	})
}

type deel struct {
	company string
	client  *http.Client
}

func (s *deel) Company() string { return s.company }

var (
	// One job object, after unescaping \" -> ". Fields appear in this order.
	deelJobRE = regexp.MustCompile(`"ashby_id":"([0-9a-f-]{36})","job_id":"[^"]*","title":"([^"]*)"` +
		`.*?"location_name":"([^"]*)".*?"is_listed":(true|false).*?"ashby_published_date":"([^"]*)"` +
		`.*?"all_locations":\[([^\]]*)\]`)
	deelQuotedRE = regexp.MustCompile(`"([^"]+)"`)
)

func (s *deel) Fetch(ctx context.Context) ([]model.Job, error) {
	body, err := fetchHTML(ctx, s.client, "https://www.deel.com/careers/", customListBodyLimit)
	if err != nil {
		return nil, err
	}
	page := strings.ReplaceAll(string(body), `\"`, `"`)
	seen := map[string]bool{}
	var jobs []model.Job
	for _, m := range deelJobRE.FindAllStringSubmatch(page, -1) {
		id, title, mainLocation, listed, published, allLocations := m[1], m[2], m[3], m[4], m[5], m[6]
		if seen[id] || listed != "true" {
			continue
		}
		seen[id] = true
		// Main location first, then every other eligible country.
		places := []string{mainLocation}
		for _, q := range deelQuotedRE.FindAllStringSubmatch(allLocations, -1) {
			if q[1] != mainLocation {
				places = append(places, q[1])
			}
		}
		posted, _ := time.Parse(time.RFC3339, published)
		jobs = append(jobs, model.Job{
			ID:       "deel/" + id,
			Company:  s.company,
			Title:    strings.TrimSpace(title),
			Location: strings.Join(nonEmpty(places...), "; "),
			URL:      fmt.Sprintf("https://www.deel.com/careers/job/%s/", id),
			PostedAt: posted,
		})
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("deel: no job entries found on the careers page (layout changed?)")
	}
	return jobs, nil
}
