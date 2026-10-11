package source

// Factorial ATS career site (no auth). The page lists every job as an
// <li class='job-offer-item' ...> with the job URL and a location id; the
// location filter's <option value='id'>Name</option> maps ids to names.
//
//	GET https://{host}/
//
// Config:
//
//	- name: Factorial
//	  source: factorial
//	  params:
//	    host: careers.factorial.com   # the company's Factorial career site

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"

	"jobwatch/internal/model"
	"jobwatch/internal/params"
)

func init() {
	Register("factorial", func(company string, p params.Map, client *http.Client) (Source, error) {
		host, err := p.Require("host")
		if err != nil {
			return nil, err
		}
		return &factorial{company: company, host: strings.ToLower(strings.TrimSpace(host)), client: client}, nil
	})
}

type factorial struct {
	company, host string
	client        *http.Client
}

func (s *factorial) Company() string { return s.company }

var (
	factorialItemRE   = regexp.MustCompile(`(?s)<li class='job-offer-item[^>]*>.*?</li>`)
	factorialURLRE    = regexp.MustCompile(`data-job-postings-url='([^']+)'`)
	factorialLocRE    = regexp.MustCompile(`data-location-id='(\d+)'`)
	factorialRemoteRE = regexp.MustCompile(`data-is-remote='true'`)
	factorialTitleRE  = regexp.MustCompile(`(?s)<span class='md:w-3/6'>\s*<div[^>]*>(.*?)</div>`)
	factorialOptionRE = regexp.MustCompile(`<option[^>]*value=['"](\d+)['"][^>]*>([^<]+)</option>`)
	factorialIDRE     = regexp.MustCompile(`-(\d+)$`)
)

func (s *factorial) Fetch(ctx context.Context) ([]model.Job, error) {
	url := fmt.Sprintf("https://%s/", s.host)
	body, err := fetchHTML(ctx, s.client, url, customListBodyLimit)
	if err != nil {
		return nil, err
	}
	page := string(body)

	// Location id -> name, from the filter dropdown.
	places := map[string]string{}
	for _, option := range factorialOptionRE.FindAllStringSubmatch(page, -1) {
		places[option[1]] = strings.TrimSpace(html.UnescapeString(option[2]))
	}

	var jobs []model.Job
	for _, item := range factorialItemRE.FindAllString(page, -1) {
		link := factorialURLRE.FindStringSubmatch(item)
		title := factorialTitleRE.FindStringSubmatch(item)
		if link == nil || title == nil {
			continue
		}
		location := ""
		if loc := factorialLocRE.FindStringSubmatch(item); loc != nil {
			location = places[loc[1]]
		}
		if factorialRemoteRE.MatchString(item) {
			location = strings.Join(nonEmpty(location, "Remote"), "; ")
		}
		id := link[1]
		if m := factorialIDRE.FindStringSubmatch(link[1]); m != nil {
			id = m[1]
		}
		jobs = append(jobs, model.Job{
			ID:       fmt.Sprintf("factorial/%s/%s", s.host, id),
			Company:  s.company,
			Title:    strings.TrimSpace(html.UnescapeString(title[1])),
			Location: location,
			URL:      link[1],
		})
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("factorial %s: no job-offer-item entries found", s.host)
	}
	return jobs, nil
}
