package source

// Recruiterflow hosted careers page (no auth). The page embeds the job list
// as JSON grouped by department: `window.jobsList = {"department": [[name, [jobs...]], ...]};`
//
//	GET https://recruiterflow.com/{company_slug}/jobs
//
// Config:
//
//	- name: Marfeel
//	  source: recruiterflow
//	  params:
//	    company_slug: marfeel

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
	Register("recruiterflow", func(company string, p params.Map, client *http.Client) (Source, error) {
		slug, err := p.Require("company_slug")
		if err != nil {
			return nil, err
		}
		return &recruiterflow{company: company, slug: strings.TrimSpace(slug), client: client}, nil
	})
}

type recruiterflow struct {
	company, slug string
	client        *http.Client
}

func (s *recruiterflow) Company() string { return s.company }

var recruiterflowDataRE = regexp.MustCompile(`(?s)window\.jobsList\s*=\s*(\{.*?\});\s*\n`)

func (s *recruiterflow) Fetch(ctx context.Context) ([]model.Job, error) {
	url := fmt.Sprintf("https://recruiterflow.com/%s/jobs", s.slug)
	body, err := fetchHTML(ctx, s.client, url, customListBodyLimit)
	if err != nil {
		return nil, err
	}
	match := recruiterflowDataRE.FindSubmatch(body)
	if match == nil {
		return nil, fmt.Errorf("recruiterflow %s: no window.jobsList on %s", s.slug, url)
	}
	// {"department": [["Business", [ {job}, ... ]], ...]}
	var data struct {
		Department [][]json.RawMessage `json:"department"`
	}
	if err := json.Unmarshal(match[1], &data); err != nil {
		return nil, fmt.Errorf("recruiterflow %s: parse jobs: %w", s.slug, err)
	}
	var jobs []model.Job
	seen := map[int]bool{}
	for _, group := range data.Department {
		if len(group) < 2 {
			continue
		}
		var list []struct {
			ID         int    `json:"job_id"`
			Name       string `json:"job_name"`
			Details    string `json:"details"` // location
			RemoteType string `json:"remote_type"`
			Employment string `json:"employment_type"`
			LastOpened string `json:"last_opened"`
			ApplyLink  string `json:"apply_link"`
		}
		if err := json.Unmarshal(group[1], &list); err != nil {
			continue
		}
		for _, job := range list {
			if seen[job.ID] {
				continue // a job can be listed under "Any" and its own department
			}
			seen[job.ID] = true
			location := strings.Join(nonEmpty(job.Details, job.RemoteType), "; ")
			opened, _ := time.Parse("2006-01-02T15:04:05-0700", job.LastOpened)
			jobs = append(jobs, model.Job{
				ID:             fmt.Sprintf("recruiterflow/%s/%d", s.slug, job.ID),
				Company:        s.company,
				Title:          strings.TrimSpace(job.Name),
				Location:       location,
				URL:            "https://recruiterflow.com/" + strings.TrimPrefix(job.ApplyLink, "/"),
				EmploymentType: job.Employment,
				PostedAt:       opened,
			})
		}
	}
	return jobs, nil
}
