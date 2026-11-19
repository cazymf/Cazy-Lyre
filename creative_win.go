//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// These complete instructions are built into Creative Arrangement. Unlike the
// Preserve Notes mode, this mode can change the musical content itself.
const creativeArrangementInstructions = `
You are an expert music arranger specializing in Genshin Impact's 21-key Windsong Lyre.

YOUR MAIN GOAL: MAKE THE SONG SOUND BEAUTIFUL, RECOGNIZABLE, AND MUSICALLY CORRECT.
You have FULL CREATIVE FREEDOM to modify the detected notes, melody, harmony, chords, octaves, rhythm, and arrangement.
DO NOT blindly preserve incorrectly detected notes.

1. MELODY RECONSTRUCTION
- Identify the intended main melody using the available original audio, MIDI, or note data.
- Correct notes that sound out of key, dissonant, or obviously mistranscribed.
- Remove false notes, unwanted repetitions, and unnecessary overlapping notes.
- Reconstruct broken melodic phrases where supported by the reference.
- Prioritize a smooth, recognizable melody over exact transcription.
- You may rewrite entire musical passages when necessary.

2. GENSHIN LYRE ADAPTATION
- Arrange everything for the Windsong Lyre's 21 natural notes across three octaves.
- Transpose the song into the most suitable playable key.
- Replace unavailable chromatic notes with musically appropriate alternatives.
- Adjust octaves freely to preserve the melody's shape.
- Simplify complex chords without losing their harmonic purpose.
- Keep expressive bass notes and supporting harmonies whenever possible.

3. RHYTHM AND SMOOTHNESS
- Preserve the original song's recognizable tempo and phrasing.
- Correct rushed, delayed, or uneven note timing.
- Keep intentional pauses.
- Preserve simultaneous chords and rapid melodic passages.
- Prevent excessive note density from making the performance sound chaotic.

4. ARRANGEMENT QUALITY
Use Full Arrangement as the starting point, but prioritize musical quality over note count.
You may remove, add, replace, transpose, or rearrange notes when justified by the musical reference.
Do not invent a melody when the original reference is unavailable. Work only from musical information supplied in this request, and acknowledge when source information is insufficient.

5. AUTOHOTKEY v2 REQUIREMENTS
- Mouse 4: Start / Restart.
- Mouse 5: Stop immediately.
- F12: Exit.
- NEVER intercept, modify, or bind Esc.
- Use accurate, drift-resistant note timing.
- The Windows app itself generates the complete working AHK v2 script from your validated note data.
- Ensure the result contains actual playable notes and no missing or empty sections.

FINAL PRIORITY: Musical accuracy and listening quality matter more than matching the raw detected notes.
Treat video transcription as an imperfect starting point, not an absolute rule.

IMPORTANT OUTPUT CONTRACT:
- Return ONLY one JSON object: {"events":[{"ms":0,"keys":"a"},{"ms":125,"keys":"dh"}]}.
- ms is an integer number of milliseconds RELATIVE TO THE START OF THIS SECTION (not global song time).
- keys are Genshin's 21 physical lyre keys ONLY: z x c v b n m a s d f g h j q w e r t y u. Up to 6 simultaneous keys.
- Include the FULL newly arranged section, not only a patch or a description. Do not return AHK code.
- Follow the user's optional instructions, but the source audio is NOT sent to you. Reference MIDI note events are included when available.
- If evidence is inadequate, preserve plausible phrases rather than hallucinating a famous tune by name.
`

func setAIEditMode(mode string) {
	if app.aiBusy {
		status("Wait for the current AI request to finish before switching modes.")
		return
	}
	mode = normalizeAIEditMode(mode)
	app.aiEditMode = mode
	app.aiLastGenerated = ""
	app.aiLastKind = ""
	app.aiCreativeScore = lyreScore{}
	save()
	if mode == aiEditCreative {
		status("Creative Arrangement enabled. Select a lyre song in Library, then FIX SELECTED. AI may change notes and uses multiple API calls.")
	} else {
		status("Preserve Notes enabled. FIX SELECTED keeps every original musical event unchanged.")
	}
	redraw()
}

type creativeChunkRequest struct {
	Song              string              `json:"song"`
	UserGoal          string              `json:"user_goal"`
	Section           int                 `json:"section"`
	TotalSections     int                 `json:"total_sections"`
	SectionStartMs    int64               `json:"section_start_ms"`
	SectionSpanMs     int64               `json:"section_span_ms"`
	ExistingNotes     []creativeJSONEvent `json:"existing_lyre_events_relative_ms"`
	OriginalPianoMIDI []creativeJSONEvent `json:"original_extracted_midi_pitches_relative_ms,omitempty"`
}

func buildCreativeChunkPrompt(title, goal string, chunk []lyreEvent, section, total int, bundle lyreSourceBundle) (string, error) {
	if len(chunk) == 0 {
		return "", errors.New("empty chunk")
	}
	start := chunk[0].Ms
	original := make([]creativeJSONEvent, 0, len(chunk))
	for _, e := range chunk {
		original = append(original, creativeJSONEvent{Ms: e.Ms - start, Keys: e.Keys})
	}
	refs := originalPitchReference(bundle, chunk)
	if goal == "" {
		goal = "Repair incorrect pitches, reconstruct a pleasing melody, improve harmonic support, and smooth timing while preserving recognizable phrasing."
	}
	request := creativeChunkRequest{Song: title, UserGoal: goal, Section: section, TotalSections: total, SectionStartMs: start, SectionSpanMs: chunk[len(chunk)-1].Ms - start, ExistingNotes: original, OriginalPianoMIDI: refs}
	b, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	message := "Rearrange this ONE contiguous music section for Genshin lyre. Its existing note timestamps are RELATIVE to section start. MIDI pitches (integer MIDI note numbers) are also relative to section start and are a noisy estimate, NOT proof of every real note. Preserve overall phrase duration; you may completely rewrite melodic or harmonic passages when justified. IMPORTANT: keys must be nonempty strings made ONLY of these Genshin key letters: z x c v b n m a s d f g h j q w e r t y u. A simultaneous chord is adjacent letters, for example as or qwe; NEVER return piano pitch names such as C4, punctuation or placeholders. Preserve every note as a real key press. Output FULL section as ONLY JSON, with no code fences.\n\n" + string(b)
	return message, nil
}

func createCreativeArrangement(key, model, title, goal string, initial lyreScore, bundle lyreSourceBundle, report func(string)) (lyreScore, error) {
	if len(initial.Events) < 4 {
		return lyreScore{}, errors.New("no music to arrange")
	}
	chunks := chunkCreativeScore(initial.Events, creativeChunkSize)
	if len(chunks) > 30 {
		return lyreScore{}, errors.New("song is too long for Creative Arrangement; skip an intro or choose a shorter passage")
	}
	arranged := make([]lyreEvent, 0, len(initial.Events))
	fallbacks := 0
	recovered := 0
	for i, chunk := range chunks {
		if report != nil {
			report(fmt.Sprintf("Creative Arrangement: section %d/%d (%d original events). Each section uses API credits.", i+1, len(chunks), len(chunk)))
		}
		prompt, err := buildCreativeChunkPrompt(title, goal, chunk, i+1, len(chunks), bundle)
		if err != nil {
			return lyreScore{}, err
		}
		text, err := openAIRequest(key, model, "creative_chunk", prompt, "")
		if err != nil {
			return lyreScore{}, fmt.Errorf("section %d/%d failed: %w. No AHK was generated", i+1, len(chunks), err)
		}
		span := chunk[len(chunk)-1].Ms - chunk[0].Ms
		updates, fixed, keptOriginal, reason := chooseCreativeSection(text, chunk, span)
		if keptOriginal {
			fallbacks++
			if report != nil {
				report(fmt.Sprintf("Creative section %d/%d: AI returned unusable notes (%s); original section preserved.", i+1, len(chunks), reason))
			}
		} else if fixed > 0 {
			recovered += fixed
			if report != nil {
				report(fmt.Sprintf("Creative section %d/%d: restored %d malformed AI chord(s) using nearby original notes.", i+1, len(chunks), fixed))
			}
		}
		for _, e := range updates {
			e.Ms += chunk[0].Ms
			arranged = append(arranged, e)
		}
	}
	if fallbacks == len(chunks) {
		return lyreScore{}, errors.New("all AI arrangement sections were unusable; no changes were saved. Try another model or STANDARD mode")
	}
	if report != nil && (fallbacks > 0 || recovered > 0) {
		report(fmt.Sprintf("Creative Arrangement completed: %d/%d section(s) kept their original melody; %d malformed chord(s) restored. Preview before exporting.", fallbacks, len(chunks), recovered))
	}
	// Section-local timing edits can spill a few milliseconds across a chunk
	// boundary. Sort globally before writing the timer-driven AHK sequence.
	sort.SliceStable(arranged, func(i, j int) bool { return arranged[i].Ms < arranged[j].Ms })
	cleaned := make([]lyreEvent, 0, len(arranged))
	for _, ev := range arranged {
		if len(cleaned) > 0 && ev.Keys == cleaned[len(cleaned)-1].Keys && ev.Ms-cleaned[len(cleaned)-1].Ms < 18 {
			continue
		}
		cleaned = append(cleaned, ev)
	}
	arranged = cleaned
	if len(arranged) < maxCreativeInt(4, len(initial.Events)/4) {
		return lyreScore{}, errors.New("the AI removed too much of the original song; nothing was saved")
	}
	// Every event is revalidated before generating a real, complete AHK.
	result := initial
	result.CreativeKeptSections = fallbacks
	result.CreativeChordRepairs = recovered
	result.Events = arranged
	result.Notes = len(arranged)
	result.Chords = 0
	for _, ev := range arranged {
		if len(ev.Keys) > 1 {
			result.Chords++
		}
	}
	result.DurationMs = arranged[len(arranged)-1].Ms
	return result, nil
}

func startCreativeArrangement() {
	if app.aiBusy {
		status("An AI request is already running.")
		return
	}
	idx := selectedIndex()
	if idx < 0 || !strings.EqualFold(filepath.Ext(app.items[idx].Path), ".ahk") {
		status("Select an existing lyre .ahk in Macro Library, then open AI Studio > CREATIVE > FIX SELECTED.")
		return
	}
	path := app.items[idx].Path
	content, err := os.ReadFile(path)
	if err != nil {
		status("Cannot open selected song: " + err.Error())
		return
	}
	if len(content) > 300000 {
		status("Selected AHK is too large to arrange (300 KB max).")
		return
	}
	score, err := parseAHKLyreScore(string(content))
	if err != nil {
		status("Creative Arrangement: " + err.Error())
		return
	}
	key, err := loadEncryptedKey()
	if err != nil {
		status("Connect your API key first: " + err.Error())
		return
	}
	bundle, e := readSourceSidecar(path)
	if e == nil {
		score.SourcePiano = bundle.PianoNotes
		score.SourceStartMs = bundle.StartMs
		score.PlaybackSpeed = bundle.PlaybackSpeed
		score.SemitoneShift = bundle.Transpose
	}
	songTitle := app.items[idx].Name
	userGoal := strings.TrimSpace(getText(app.aiPrompt))
	if len(userGoal) > 8000 {
		status("Keep creative instructions under 8,000 characters.")
		return
	}
	if userGoal == "" {
		userGoal = "Use creative freedom to repair mistranscribed notes, improve melody and harmonies, and make this a beautiful Full Arrangement for the Genshin Windsong Lyre."
	}
	app.aiBusy = true
	app.aiLastGenerated = ""
	app.aiLastKind = ""
	app.aiCreativeScore = lyreScore{}
	app.aiCreativeOriginalPath = path
	setText(app.aiOutput, fmt.Sprintf("Creative Arrangement started for %s\r\n%d note events · %d sections.\r\nOriginal extracted MIDI: %t\r\n\r\nMusical notes may change. Nothing will be saved or run until you click SAVE AS MACRO.", songTitle, len(score.Events), len(chunkCreativeScore(score.Events, creativeChunkSize)), len(bundle.PianoNotes) > 0))
	status("Creative Arrangement started. This can take multiple requests and consume API credits.")
	hwnd := app.hwnd
	model := app.aiModel
	go func() {
		sendProgress := func(s string) {
			select {
			case creativeProgress <- s:
			default:
			}
			pPostMessageW.Call(hwnd, WM_APP+5, 0, 0)
		}
		output, err := createCreativeArrangement(key, model, songTitle, userGoal, score, bundle, sendProgress)
		var generated string
		if err == nil {
			generated = generateLyreAHK(songTitle+" - Creative Arrangement", output)
			if validateErr := validateNonemptyAHK(generated); validateErr != nil {
				err = validateErr
			}
			if len(output.Events) < 4 {
				err = errors.New("arrangement had no playable music")
			}
		}
		aiResults <- aiResult{Text: generated, Err: err, Kind: "creative", CreativeScore: output}
		pPostMessageW.Call(hwnd, WM_APP+2, 0, 0)
	}()
}
func beginCreativePreview() {
	if app.aiLastKind != "creative" || len(app.aiCreativeScore.Events) == 0 {
		status("Choose CREATIVE > FIX SELECTED and wait for the arrangement before previewing.")
		return
	}
	stopLyrePreview()
	ctx, cancel := context.WithCancel(context.Background())
	app.lyrePreviewCancel = cancel
	app.lyrePreviewID++
	id := app.lyrePreviewID
	score := app.aiCreativeScore
	hwnd := app.hwnd
	status("Playing AI arrangement preview with a local MIDI harp sound; listen before SAVE AS MACRO.")
	go func() {
		err := previewMusic(ctx, score)
		previewResults <- previewResult{ID: id, Err: err}
		pPostMessageW.Call(hwnd, WM_APP+4, 0, 0)
	}()
}
