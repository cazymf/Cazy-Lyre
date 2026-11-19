//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unsafe"
)

var songSearchResults = make(chan songSearchResult, 2)

func startSongNameSearch() {
	if app.songSearchBusy {
		status("Online search is already in progress.")
		return
	}
	query := strings.TrimSpace(getText(app.songQuery))
	if len(query) < 2 || len(query) > 180 {
		status("Enter a song title (2–180 characters) first.")
		return
	}
	key, err := loadEncryptedKey()
	if err != nil {
		status("Connect your OpenAI API key in AI Studio before searching: " + err.Error())
		return
	}
	app.songSearchBusy = true
	app.songSearchSummary = "Searching for the best available sources: MIDI, Genshin lyre covers, and piano tutorials..."
	app.songSearchSources = nil
	status("Searching online for MIDI, Genshin lyre videos, and piano tutorials; uses API credits...")
	hwnd := app.hwnd
	go func() {
		result := searchSongOnline(key, query)
		songSearchResults <- result
		pPostMessageW.Call(hwnd, WM_APP+6, 0, 0)
	}()
}

func searchSongOnline(key, title string) songSearchResult {
	prompt := "Search the live web for the best available playable sources for the following requested song. Look for any combination of legitimate MIDI arrangements, public music sheets, Synthesia-style piano tutorials, and Genshin Impact Windsong Lyre cover or tutorial videos that clearly show the keys or notation. The goal is to help a converter learn the song from the strongest available source or combination of sources. Prefer direct MIDI pages first when available, but also include strong Genshin lyre and piano-tutorial sources when useful. The app needs a playable MIDI, MP3 or MP4 source; a song name alone cannot supply note timings. Do not fabricate URLs, download files, bypass paywalls or claim a source is free if unsure. Explain briefly which source type looks strongest and why, and provide citations. Song title: " + title
	payload := map[string]interface{}{
		"model": "gpt-6.1-sol", "input": prompt,
		"instructions": "You are Cazy Lyre's universal source finder. Actually search the internet. Only cite sources you located. Never invent MIDI files, videos, or download links. Prefer useful playable sources in this rough order: strong MIDI files, then good Genshin lyre tutorials or covers, then Synthesia or piano tutorials, then other playable sources. Mention the strongest source type and whether combining sources may help.",
		"tools":        []map[string]string{{"type": "web_search"}}, "tool_choice": "required",
		"reasoning": map[string]string{"effort": "low"}, "max_output_tokens": 1400, "store": false,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return songSearchResult{Err: err}
	}
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/responses", bytes.NewReader(data))
	if err != nil {
		return songSearchResult{Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 115 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return songSearchResult{Err: fmt.Errorf("Search connection failed: %w", err)}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	if err != nil {
		return songSearchResult{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var problem apiErrorEnvelope
		if json.Unmarshal(body, &problem) == nil && problem.Error.Message != "" {
			return songSearchResult{Err: fmt.Errorf("OpenAI search HTTP %d: %s", resp.StatusCode, problem.Error.Message)}
		}
		return songSearchResult{Err: fmt.Errorf("OpenAI search HTTP %d; check selected model, credits, or web-search access", resp.StatusCode)}
	}
	result, searched, err := parseSongSearchResponse(body)
	if err != nil {
		return songSearchResult{Err: err}
	}
	if !searched {
		return songSearchResult{Err: errors.New("OpenAI did not perform the requested web search; no search results were used")}
	}
	if len(result.Sources) == 0 {
		return songSearchResult{Summary: result.Summary, Err: errors.New("Search returned no cited source links. Try the full song name and artist")}
	}
	return result
}

func showSongSearch() {
	if app.songQuery == 0 {
		return
	}
	current := strings.TrimSpace(getText(app.lyreSong))
	if current != "" && getText(app.songQuery) == "" {
		setText(app.songQuery, current)
	}
	switchPage("songsearch")
	pSetFocus.Call(app.songQuery)
	status("Type a song and click SEARCH ONLINE. Results can include MIDI, Genshin lyre videos, and piano tutorials. Open a source in your browser, then import the downloaded audio/MIDI/video file to convert.")
}
func openSongSource(index int) {
	if index < 0 || index >= len(app.songSearchSources) {
		return
	}
	link := app.songSearchSources[index].URL
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		status("Search source link is invalid")
		return
	}
	verb, target := utf("open"), utf(link)
	result, _, _ := pShellExecuteW.Call(app.hwnd, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, SW_SHOWNORMAL)
	if result <= 32 {
		status("Could not open browser for this source")
	} else {
		status("Source opened in your browser. Download a MIDI or tutorial from the source, then import it into Cazy Lyre.")
	}
}
func paintSongSearch(hdc uintptr, w, h int) {
	rounded(hdc, 283, 282, w-316, 123, palette.panel, palette.border, 16)
	textDraw(hdc, "FIND A SONG — ONLINE SOURCE SEARCH", 304, 287, w-356, 23, palette.accent2, app.microFont, DT_LEFT)
	textDraw(hdc, "Song / artist / soundtrack", 304, 320, 167, 24, palette.white, app.subFont, DT_LEFT)
	button(hdc, "song-search-go", "SEARCH ONLINE", w-241, 317, 185, 34, 1)
	button(hdc, "song-search-import", "IMPORT DOWNLOADED FILE", 304, 365, 242, 31, 2)
	button(hdc, "song-search-back", "BACK TO CONVERTER", 555, 365, 218, 31, 0)
	label := "Web search uses OpenAI API credits. It can suggest MIDI files, Genshin lyre videos, and piano tutorials. Results open in the browser; they are not downloaded automatically."
	textDraw(hdc, label, 304, 410, w-355, 40, palette.muted, app.subFont, DT_LEFT)
	if app.songSearchBusy {
		textDraw(hdc, "Searching the web for MIDI, lyre covers, and piano tutorials...", 304, 445, w-350, 30, palette.accent2, app.itemFont, DT_LEFT)
		return
	}
	if app.songSearchSummary == "" {
		textDraw(hdc, "Enter a song title above, then click SEARCH ONLINE for MIDI, Genshin lyre, and piano results.", 304, 452, w-355, 29, palette.muted, app.itemFont, DT_LEFT)
		return
	}
	summary := strings.ReplaceAll(app.songSearchSummary, "\n", " ")
	textDraw(hdc, byteshort(summary, 165), 304, 443, w-357, 52, palette.white, app.subFont, DT_LEFT)
	if len(app.songSearchSources) > 0 {
		textDraw(hdc, "CITED SOURCES — CLICK TO OPEN", 304, 493, w-354, 25, palette.accent2, app.microFont, DT_LEFT)
	}
	maxRows := (h - 75 - 525) / 54
	if maxRows > 5 {
		maxRows = 5
	}
	if maxRows < 0 {
		maxRows = 0
	}
	for i := 0; i < len(app.songSearchSources) && i < maxRows; i++ {
		s := app.songSearchSources[i]
		y := 519 + i*54
		rounded(hdc, 299, y, w-349, 49, palette.panel, palette.border, 9)
		textDraw(hdc, byteshort(s.Title, 75), 311, y+4, w-380, 20, palette.white, app.itemFont, DT_LEFT)
		textDraw(hdc, byteshort(s.URL, 100), 311, y+26, w-380, 16, palette.accent2, app.microFont, DT_LEFT)
		hit(fmt.Sprintf("song-source:%d", i), 299, y, w-349, 49)
	}
}
