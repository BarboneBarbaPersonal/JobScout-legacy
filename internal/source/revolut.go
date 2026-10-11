package source

// Revolut careers page (no auth). The Next.js page embeds every position in
// its __NEXT_DATA__ script (props.pageProps.positions). Revolut only serves
// that data to browser-like requests, so this source sends browser headers.
//
//	GET https://www.revolut.com/careers/
//
// Revolut's bot protection rejects Go's TLS fingerprint (403) while curl gets
// through, so the workflow downloads the page with curl first and this source
// reads that file when `file` is set (falling back to a direct request).
//
// Config:
//
//	- name: Revolut
//	  source: revolut
//	  params:
//	    file: revolut-careers.html   # optional, saved by the workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"jobwatch/internal/model"
	"jobwatch/internal/params"
)

func init() {
	Register("revolut", func(company string, p params.Map, client *http.Client) (Source, error) {
		return &revolut{company: company, file: p.Get("file"), client: client}, nil
	})
}

type revolut struct {
	company string
	file    string // optional pre-downloaded page
	client  *http.Client
}

func (s *revolut) Company() string { return s.company }

var (
	revolutDataRE = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)
	slugSplitRE   = regexp.MustCompile(`[^a-z0-9]+`)
)

const revolutURL = "https://www.revolut.com/careers/"

func (s *revolut) Fetch(ctx context.Context) ([]model.Job, error) {
	body, err := s.page(ctx)
	if err != nil {
		return nil, err
	}
	return s.parse(body)
}

// page returns the careers HTML: the pre-downloaded file when it exists and
// looks complete, otherwise a direct request.
func (s *revolut) page(ctx context.Context) ([]byte, error) {
	if s.file != "" {
		if body, err := os.ReadFile(s.file); err == nil && revolutDataRE.Match(body) {
			return body, nil
		}
	}
	const url = revolutURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-GB,en;q=0.9")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, customListBodyLimit))
}

func (s *revolut) parse(body []byte) ([]model.Job, error) {
	match := revolutDataRE.FindSubmatch(body)
	if match == nil {
		return nil, fmt.Errorf("revolut: no __NEXT_DATA__ on %s (page layout changed?)", revolutURL)
	}
	var data struct {
		Props struct {
			PageProps struct {
				Positions []struct {
					ID        string `json:"id"`
					Text      string `json:"text"` // the job title
					Team      string `json:"team"`
					Locations []struct {
						Name    string `json:"name"`
						Country string `json:"country"`
					} `json:"locations"`
				} `json:"positions"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal(match[1], &data); err != nil {
		return nil, fmt.Errorf("revolut: parse __NEXT_DATA__: %w", err)
	}
	positions := data.Props.PageProps.Positions
	if len(positions) == 0 {
		return nil, fmt.Errorf("revolut: no positions in __NEXT_DATA__")
	}
	jobs := make([]model.Job, 0, len(positions))
	for _, pos := range positions {
		// Location text: "Barcelona, Spain; Spain - Remote, Spain; ..."
		var places []string
		for _, loc := range pos.Locations {
			places = append(places, strings.Join(nonEmpty(loc.Name, loc.Country), ", "))
		}
		// Job pages live at /careers/position/<title-slug>-<id>/
		slug := strings.Trim(slugSplitRE.ReplaceAllString(strings.ToLower(pos.Text), "-"), "-")
		jobs = append(jobs, model.Job{
			ID:       "revolut/" + pos.ID,
			Company:  s.company,
			Title:    strings.TrimSpace(pos.Text),
			Location: strings.Join(places, "; "),
			URL:      fmt.Sprintf("https://www.revolut.com/careers/position/%s-%s/", slug, pos.ID),
		})
	}
	return jobs, nil
}
