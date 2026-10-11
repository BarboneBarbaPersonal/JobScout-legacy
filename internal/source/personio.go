package source

// Personio public job feed (no auth). Every Personio career site serves:
//
//	GET https://{host}/xml?language=en
//
// Config:
//
//	- name: Hornetsecurity
//	  source: personio
//	  params:
//	    host: hornetsecurity.jobs.personio.de   # the career site's host, no scheme

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"jobwatch/internal/htmltext"
	"jobwatch/internal/model"
	"jobwatch/internal/params"
)

func init() {
	Register("personio", func(company string, p params.Map, client *http.Client) (Source, error) {
		host, err := p.Require("host")
		if err != nil {
			return nil, err
		}
		return &personio{company: company, host: strings.ToLower(strings.TrimSpace(host)), client: client}, nil
	})
}

type personio struct {
	company string
	host    string
	client  *http.Client
}

func (s *personio) Company() string { return s.company }

type personioFeed struct {
	Positions []struct {
		ID                string   `xml:"id"`
		Name              string   `xml:"name"`
		Office            string   `xml:"office"`
		AdditionalOffices []string `xml:"additionalOffices>office"`
		Schedule          string   `xml:"schedule"`
		CreatedAt         string   `xml:"createdAt"`
		Descriptions      []struct {
			Name  string `xml:"name"`
			Value string `xml:"value"` // HTML
		} `xml:"jobDescriptions>jobDescription"`
	} `xml:"position"`
}

func (s *personio) Fetch(ctx context.Context) ([]model.Job, error) {
	body, err := fetchHTML(ctx, s.client, fmt.Sprintf("https://%s/xml?language=en", s.host), customListBodyLimit)
	if err != nil {
		return nil, err
	}
	var feed personioFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("personio %s: parse xml: %w", s.host, err)
	}
	jobs := make([]model.Job, 0, len(feed.Positions))
	for _, pos := range feed.Positions {
		offices := append([]string{pos.Office}, pos.AdditionalOffices...)
		var parts []string
		for _, d := range pos.Descriptions {
			parts = append(parts, d.Name+"\n"+htmltext.ToText(d.Value))
		}
		posted, _ := time.Parse(time.RFC3339, strings.TrimSpace(pos.CreatedAt))
		jobs = append(jobs, model.Job{
			ID:             fmt.Sprintf("personio/%s/%s", s.host, strings.TrimSpace(pos.ID)),
			Company:        s.company,
			Title:          strings.TrimSpace(pos.Name),
			Location:       strings.Join(offices, "; "),
			URL:            fmt.Sprintf("https://%s/job/%s", s.host, strings.TrimSpace(pos.ID)),
			EmploymentType: pos.Schedule,
			Description:    strings.Join(parts, "\n\n"),
			PostedAt:       posted,
		})
	}
	return jobs, nil
}
