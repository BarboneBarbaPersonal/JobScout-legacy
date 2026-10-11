package source

// Generic "careers page that links to each job" source (no auth). Reads one
// HTML page and turns every link whose href matches `link` into a job.
//
// The `link` regex may use named groups:
//   (?P<location>...)  location text taken from the URL (dashes become spaces)
//   (?P<title>...)     title taken from the URL, used when the link text is empty
// Otherwise the title is the link text and the location is the `location` param.
//
// Config:
//
//	- name: Coralogix
//	  source: htmllinks
//	  params:
//	    url: https://coralogix.com/careers/
//	    link: 'https://coralogix\.com/careers/co/(?P<location>[^/]+)/[^/]+/(?P<title>[^/]+)/all/'
//	- name: Doist
//	  source: htmllinks
//	  params:
//	    url: https://www.todoist.com/careers
//	    link: '/careers/[0-9a-f-]{36}-[a-z0-9-]+'
//	    location: Remote (worldwide)

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"jobwatch/internal/htmltext"
	"jobwatch/internal/model"
	"jobwatch/internal/params"
)

func init() {
	Register("htmllinks", func(company string, p params.Map, client *http.Client) (Source, error) {
		pageURL, err := p.Require("url")
		if err != nil {
			return nil, err
		}
		pattern, err := p.Require("link")
		if err != nil {
			return nil, err
		}
		link, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("param \"link\": %w", err)
		}
		textLocation, err := p.Bool("text_location", false)
		if err != nil {
			return nil, err
		}
		return &htmlLinks{company: company, url: pageURL, link: link, location: p.Get("location"),
			textLocation: textLocation, client: client}, nil
	})
}

type htmlLinks struct {
	company, url, location string
	textLocation           bool // link text = title line, then location line(s)
	link                   *regexp.Regexp
	client                 *http.Client
}

func (s *htmlLinks) Company() string { return s.company }

var anchorRE = regexp.MustCompile(`(?is)<a\b[^>]*\bhref=["']([^"']+)["'][^>]*>(.*?)</a>`)

// fromSlug turns "team-lead-technical-account-manager-us" into readable words.
func fromSlug(slug string) string {
	slug, _ = url.PathUnescape(slug)
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(slug, "-", " "), "_", " "))
}

func (s *htmlLinks) Fetch(ctx context.Context) ([]model.Job, error) {
	body, err := fetchHTML(ctx, s.client, s.url, customListBodyLimit)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(s.url)
	seen := map[string]bool{}
	var jobs []model.Job
	for _, anchor := range anchorRE.FindAllStringSubmatch(string(body), -1) {
		href := html.UnescapeString(anchor[1])
		if resolved, err := base.Parse(href); err == nil {
			href = resolved.String()
		}
		match := s.link.FindStringSubmatch(href)
		if match == nil || seen[href] {
			continue
		}
		seen[href] = true
		title := strings.TrimSpace(htmltext.ToText(anchor[2]))
		location := s.location
		for i, name := range s.link.SubexpNames() {
			switch {
			case name == "location" && match[i] != "":
				location = strings.Join(nonEmpty(fromSlug(match[i]), s.location), "; ")
			case name == "title" && title == "":
				title = fromSlug(match[i])
			}
		}
		// Link text can include extra lines (team, city): the first line is the
		// title; with text_location the remaining lines are the location.
		lines := nonEmpty(strings.Split(title, "\n")...)
		title = ""
		if len(lines) > 0 {
			title = lines[0]
			if s.textLocation && len(lines) > 1 {
				location = strings.Join(nonEmpty(append(lines[1:], s.location)...), "; ")
			}
		}
		if title == "" {
			continue
		}
		jobs = append(jobs, model.Job{
			ID:       "htmllinks/" + strings.TrimPrefix(strings.TrimPrefix(href, "https://"), "http://"),
			Company:  s.company,
			Title:    title,
			Location: location,
			URL:      href,
		})
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("htmllinks %s: no links matched %q", s.url, s.link.String())
	}
	return jobs, nil
}
