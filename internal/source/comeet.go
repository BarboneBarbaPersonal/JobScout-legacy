package source

// Comeet hosted careers page (no auth). The page embeds every open position
// as JSON in a script: `COMPANY_POSITIONS_DATA = [...];`
//
//	GET https://www.comeet.com/jobs/{company_slug}/{company_uid}
//
// Config:
//
//	- name: groundcover
//	  source: comeet
//	  params:
//	    company_slug: groundcover
//	    company_uid: "88.008"     # from a comeet.com/jobs/... link on their careers page

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"jobwatch/internal/model"
	"jobwatch/internal/params"
)

func init() {
	Register("comeet", func(company string, p params.Map, client *http.Client) (Source, error) {
		slug, err := p.Require("company_slug")
		if err != nil {
			return nil, err
		}
		uid, err := p.Require("company_uid")
		if err != nil {
			return nil, err
		}
		return &comeet{company: company, slug: strings.TrimSpace(slug), uid: strings.TrimSpace(uid), client: client}, nil
	})
}

type comeet struct {
	company, slug, uid string
	client             *http.Client
}

func (s *comeet) Company() string { return s.company }

var comeetDataRE = regexp.MustCompile(`(?s)COMPANY_POSITIONS_DATA\s*=\s*(\[.*?\]);`)

func (s *comeet) Fetch(ctx context.Context) ([]model.Job, error) {
	url := fmt.Sprintf("https://www.comeet.com/jobs/%s/%s", s.slug, s.uid)
	body, err := fetchHTML(ctx, s.client, url, customListBodyLimit)
	if err != nil {
		return nil, err
	}
	match := comeetDataRE.FindSubmatch(body)
	if match == nil {
		return nil, fmt.Errorf("comeet %s: no COMPANY_POSITIONS_DATA on %s", s.slug, url)
	}
	var positions []struct {
		UID      string `json:"uid"`
		Name     string `json:"name"`
		URL      string `json:"url_active_page"`
		Updated  string `json:"time_updated"`
		Location struct {
			Name     string `json:"name"`
			City     string `json:"city"`
			Country  string `json:"country"`
			IsRemote bool   `json:"is_remote"`
		} `json:"location"`
		EmploymentType string `json:"employment_type"`
	}
	if err := json.Unmarshal(match[1], &positions); err != nil {
		return nil, fmt.Errorf("comeet %s: parse positions: %w", s.slug, err)
	}
	jobs := make([]model.Job, 0, len(positions))
	for _, pos := range positions {
		// "Boston, US" plus "Remote" when the role allows it.
		place := strings.Join(nonEmpty(pos.Location.City, pos.Location.Country), ", ")
		if place == "" {
			place = pos.Location.Name
		}
		if pos.Location.IsRemote {
			place += "; Remote"
		}
		updated, _ := time.Parse(time.RFC3339, pos.Updated)
		jobs = append(jobs, model.Job{
			ID:             fmt.Sprintf("comeet/%s/%s", s.slug, pos.UID),
			Company:        s.company,
			Title:          strings.TrimSpace(pos.Name),
			Location:       strings.TrimPrefix(place, "; "),
			URL:            pos.URL,
			EmploymentType: pos.EmploymentType,
			PostedAt:       updated,
		})
	}
	return jobs, nil
}

// nonEmpty keeps the trimmed, non-empty strings.
func nonEmpty(values ...string) []string {
	var out []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
