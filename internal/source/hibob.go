package source

// HiBob career site API (no auth beyond the company header the site sends).
//
//	GET https://{subdomain}.careers.hibob.com/api/job-ad
//	    header companyidentifier: {subdomain}
//
// Config:
//
//	- name: Kiteworks
//	  source: hibob
//	  params:
//	    subdomain: kiteworks    # from {subdomain}.careers.hibob.com

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"jobwatch/internal/htmltext"
	"jobwatch/internal/model"
	"jobwatch/internal/params"
)

func init() {
	Register("hibob", func(company string, p params.Map, client *http.Client) (Source, error) {
		sub, err := p.Require("subdomain")
		if err != nil {
			return nil, err
		}
		return &hibob{company: company, sub: strings.ToLower(strings.TrimSpace(sub)), client: client}, nil
	})
}

type hibob struct {
	company, sub string
	client       *http.Client
}

func (s *hibob) Company() string { return s.company }

func (s *hibob) Fetch(ctx context.Context) ([]model.Job, error) {
	url := fmt.Sprintf("https://%s.careers.hibob.com/api/job-ad", s.sub)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("companyidentifier", s.sub)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("GET %s: %s: %s", url, resp.Status, strings.TrimSpace(string(snippet)))
	}
	var payload struct {
		Jobs []struct {
			ID             string `json:"id"`
			Title          string `json:"title"`
			Site           string `json:"site"`
			Country        string `json:"country"`
			EmploymentType string `json:"employmentType"`
			WorkspaceType  string `json:"workspaceType"` // e.g. "Remote", "Hybrid"
			Description    string `json:"description"`   // HTML
		} `json:"jobAdDetails"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("GET %s: decoding response: %w", url, err)
	}
	jobs := make([]model.Job, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		location := strings.Join(nonEmpty(job.Site, job.Country), ", ")
		if strings.EqualFold(job.WorkspaceType, "remote") {
			location = strings.Join(nonEmpty(location, "Remote"), "; ")
		}
		jobs = append(jobs, model.Job{
			ID:             fmt.Sprintf("hibob/%s/%s", s.sub, job.ID),
			Company:        s.company,
			Title:          strings.TrimSpace(job.Title),
			Location:       location,
			URL:            fmt.Sprintf("https://%s.careers.hibob.com/jobs/%s", s.sub, job.ID),
			EmploymentType: job.EmploymentType,
			Description:    htmltext.ToText(job.Description),
		})
	}
	return jobs, nil
}
