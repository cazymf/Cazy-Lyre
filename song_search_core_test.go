package main

import "testing"

func TestParseSongSearchResponse(t *testing.T) {
	data := []byte(`{"status":"completed","output":[{"type":"web_search_call"},{"type":"message","content":[{"type":"output_text","text":"Try this MIDI arranger","annotations":[{"type":"url_citation","title":"MIDI Sheet","url":"https://example.org/sheet/1"},{"type":"url_citation","title":"duplicate","url":"https://example.org/sheet/1"},{"type":"url_citation","title":"bad","url":"file:///etc/passwd"}]}]}]}`)
	r, searched, err := parseSongSearchResponse(data)
	if err != nil || !searched || len(r.Sources) != 1 || r.Sources[0].Title != "MIDI Sheet" {
		t.Fatalf("bad search: %+v %t %v", r, searched, err)
	}
}
