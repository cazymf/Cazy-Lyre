package main

import (
	"encoding/json"
	"net/url"
	"strings"
)

type songSearchSource struct {
	Title string
	URL   string
}
type songSearchResult struct {
	Summary string
	Sources []songSearchSource
	Err     error
}
type searchAnnotation struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}
type searchContent struct {
	Type        string             `json:"type"`
	Text        string             `json:"text"`
	Annotations []searchAnnotation `json:"annotations"`
}
type searchOutput struct {
	Type    string          `json:"type"`
	Content []searchContent `json:"content"`
}
type searchEnvelope struct {
	Status string         `json:"status"`
	Output []searchOutput `json:"output"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
	IncompleteDetails struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

// Only use citation URLs supplied by the API. Never manufacture download links.
func parseSongSearchResponse(data []byte) (songSearchResult, bool, error) {
	var resp searchEnvelope
	if err := json.Unmarshal(data, &resp); err != nil {
		return songSearchResult{}, false, err
	}
	if resp.Error != nil {
		return songSearchResult{}, false, &songSearchError{resp.Error.Message}
	}
	result := songSearchResult{}
	searched := false
	seen := make(map[string]bool)
	for _, out := range resp.Output {
		if out.Type == "web_search_call" {
			searched = true
		}
		if out.Type != "message" {
			continue
		}
		for _, c := range out.Content {
			if c.Type != "output_text" {
				continue
			}
			result.Summary += c.Text + "\n"
			for _, a := range c.Annotations {
				if a.Type != "url_citation" {
					continue
				}
				u, err := url.Parse(a.URL)
				if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || len(a.URL) > 3000 {
					continue
				}
				uri := u.String()
				if seen[uri] {
					continue
				}
				seen[uri] = true
				title := strings.TrimSpace(a.Title)
				if title == "" {
					title = u.Hostname()
				}
				result.Sources = append(result.Sources, songSearchSource{Title: title, URL: uri})
			}
		}
	}
	result.Summary = strings.TrimSpace(result.Summary)
	return result, searched, nil
}

type songSearchError struct{ Detail string }

func (e *songSearchError) Error() string { return e.Detail }
