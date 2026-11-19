package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type pianoEvent struct {
	Tick     uint64
	Pitch    int
	Velocity int
	Channel  int
	Ms       int64
}
type tempoEvent struct {
	tick uint64
	usec uint32
}
type lyreEvent struct {
	Ms   int64
	Keys string
}
type lyreScore struct {
	AgentSpentUSD float64 // estimated Sol API usage for this Agent Pro arrangement
	AgentCalls int // successful Agent Pro calls
	CreativeKeptSections int          // AI sections retained from the original score
	CreativeChordRepairs int          // malformed AI chords repaired from original notes
	SourcePiano          []pianoEvent // original MIDI events, not exposed in exported AHK
	PlaybackSpeed        float64      // original-to-playback time scaling
	Quality              string
	Notes                int
	Chords               int
	DurationMs           int64
	SourceStartMs        int64 // Start position in the original MIDI/audio timeline.
	SemitoneShift        int
	Events               []lyreEvent
}

type midiReader struct {
	data []byte
	pos  int
}

func (r *midiReader) get() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, errors.New("unexpected end of MIDI file")
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}
func (r *midiReader) varlen() (uint64, error) {
	var n uint64
	for i := 0; i < 4; i++ {
		b, e := r.get()
		if e != nil {
			return 0, e
		}
		n = n<<7 | uint64(b&0x7f)
		if b&0x80 == 0 {
			return n, nil
		}
	}
	return 0, errors.New("bad MIDI variable length time")
}
func parseMIDI(data []byte) ([]pianoEvent, error) {
	if len(data) < 14 || string(data[0:4]) != "MThd" {
		return nil, errors.New("not a Standard MIDI File (.mid)")
	}
	length := int(binary.BigEndian.Uint32(data[4:8]))
	if length < 6 || len(data) < 8+length {
		return nil, errors.New("invalid MIDI header")
	}
	format := binary.BigEndian.Uint16(data[8:10])
	tracks := int(binary.BigEndian.Uint16(data[10:12]))
	division := binary.BigEndian.Uint16(data[12:14])
	if format > 2 || tracks < 1 || tracks > 500 || division == 0 || division&0x8000 != 0 {
		return nil, errors.New("unsupported MIDI format or SMPTE timing")
	}
	if format == 2 {
		return nil, errors.New("Type-2 MIDI sequences are not supported")
	}
	pos := 8 + length
	noteEvents := make([]pianoEvent, 0, 4096)
	tempoEvents := []tempoEvent{{tick: 0, usec: 500000}}
	for tr := 0; tr < tracks; tr++ {
		if pos+8 > len(data) || string(data[pos:pos+4]) != "MTrk" {
			return nil, fmt.Errorf("missing MIDI track %d", tr+1)
		}
		sz := int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
		pos += 8
		if sz < 0 || pos+sz > len(data) {
			return nil, errors.New("MIDI track exceeds file length")
		}
		rdr := midiReader{data: data[pos : pos+sz]}
		pos += sz
		var tick uint64
		var running byte
		for rdr.pos < len(rdr.data) {
			delta, e := rdr.varlen()
			if e != nil {
				return nil, e
			}
			tick += delta
			code, e := rdr.get()
			if e != nil {
				return nil, e
			}
			var first byte
			hasFirst := false
			if code < 0x80 {
				if running == 0 {
					return nil, errors.New("MIDI event without running status")
				}
				first = code
				hasFirst = true
				code = running
			} else if code < 0xF0 {
				running = code
			}
			if code == 0xFF {
				kind, e := rdr.get()
				if e != nil {
					return nil, e
				}
				size, e := rdr.varlen()
				if e != nil {
					return nil, e
				}
				if size > uint64(len(rdr.data)-rdr.pos) {
					return nil, errors.New("invalid MIDI meta length")
				}
				if kind == 0x51 && size == 3 {
					v := rdr.data[rdr.pos : rdr.pos+3]
					us := uint32(v[0])<<16 | uint32(v[1])<<8 | uint32(v[2])
					if us > 0 {
						tempoEvents = append(tempoEvents, tempoEvent{tick, us})
					}
				}
				rdr.pos += int(size)
				running = 0
				continue
			}
			if code == 0xF0 || code == 0xF7 {
				size, e := rdr.varlen()
				if e != nil {
					return nil, e
				}
				if size > uint64(len(rdr.data)-rdr.pos) {
					return nil, errors.New("invalid MIDI sysex length")
				}
				rdr.pos += int(size)
				running = 0
				continue
			}
			if code >= 0xF0 {
				return nil, fmt.Errorf("unsupported MIDI system message %X", code)
			}
			high := code & 0xF0
			channel := int(code & 0x0F)
			need := 2
			if high == 0xC0 || high == 0xD0 {
				need = 1
			}
			var a, b byte
			if hasFirst {
				a = first
			} else {
				a, e = rdr.get()
				if e != nil {
					return nil, e
				}
			}
			if need == 2 {
				b, e = rdr.get()
				if e != nil {
					return nil, e
				}
			}
			if high == 0x90 && b > 0 && channel != 9 {
				noteEvents = append(noteEvents, pianoEvent{Tick: tick, Pitch: int(a), Velocity: int(b), Channel: channel})
				if len(noteEvents) > 150000 {
					return nil, errors.New("too many MIDI note events")
				}
			}
		}
	}
	if len(noteEvents) == 0 {
		return nil, errors.New("MIDI does not contain melodic note-on events")
	}
	sort.SliceStable(tempoEvents, func(i, j int) bool { return tempoEvents[i].tick < tempoEvents[j].tick })
	sort.SliceStable(noteEvents, func(i, j int) bool { return noteEvents[i].Tick < noteEvents[j].Tick })
	ti := 0
	tempo := tempoEvents[0].usec
	var originTick uint64
	var accMicros float64
	for i := range noteEvents {
		t := noteEvents[i].Tick
		for ti+1 < len(tempoEvents) && tempoEvents[ti+1].tick <= t {
			next := tempoEvents[ti+1]
			accMicros += float64(next.tick-originTick) * float64(tempo) / float64(division)
			originTick = next.tick
			tempo = next.usec
			ti++
		}
		micro := accMicros + float64(t-originTick)*float64(tempo)/float64(division)
		noteEvents[i].Ms = int64(math.Round(micro / 1000))
	}
	return noteEvents, nil
}
func parseMIDIFile(path string) ([]pianoEvent, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if stat.Size() > 30*1024*1024 {
		return nil, errors.New("MIDI file is too large (30 MB max)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseMIDI(raw)
}

// 21 fixed, natural notes C3..B5 across Z-M, A-J, Q-U.
var pitchKeys = []struct {
	pitch int
	key   byte
}{
	{48, 'z'}, {50, 'x'}, {52, 'c'}, {53, 'v'}, {55, 'b'}, {57, 'n'}, {59, 'm'},
	{60, 'a'}, {62, 's'}, {64, 'd'}, {65, 'f'}, {67, 'g'}, {69, 'h'}, {71, 'j'},
	{72, 'q'}, {74, 'w'}, {76, 'e'}, {77, 'r'}, {79, 't'}, {81, 'y'}, {83, 'u'},
}

func mod12(n int) int {
	n %= 12
	if n < 0 {
		n += 12
	}
	return n
}
func isNatural(n int) bool {
	switch mod12(n) {
	case 0, 2, 4, 5, 7, 9, 11:
		return true
	}
	return false
}
func pickShift(notes []pianoEvent) int {
	bestShift := 0
	bestScore := -1e99
	for shift := -6; shift <= 6; shift++ {
		score := float64(-math.Abs(float64(shift))) * 0.6
		for _, n := range notes {
			p := n.Pitch + shift
			if isNatural(p) {
				score += 3.0
			} else {
				score -= 2.5
			}
			// Penalize unnecessary octave folding, without requiring original
			// music to have been performed in Genshin's three-octave range.
			if p < 48 || p > 83 {
				score -= 0.2
			}
		}
		if score > bestScore {
			bestScore = score
			bestShift = shift
		}
	}
	return bestShift
}
func nearestLyreKey(pitch int) byte {
	// First fold into the lyre's range by octave, then choose nearest diatonic.
	for pitch < 48 {
		pitch += 12
	}
	for pitch > 83 {
		pitch -= 12
	}
	bestDist := 999
	best := byte('a')
	for _, candidate := range pitchKeys {
		d := candidate.pitch - pitch
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			best = candidate.key
		}
	}
	return best
}

const (
	qualityClean    = "clean"
	qualityBalanced = "balanced"
	qualityFull     = "full"
)

func normalizeLyreQuality(s string) string {
	switch s {
	case qualityClean, qualityBalanced, qualityFull:
		return s
	default:
		return qualityFull
	}
}
func qualityDisplay(s string) string {
	switch normalizeLyreQuality(s) {
	case qualityClean:
		return "Clean Melody"
	case qualityBalanced:
		return "Balanced"
	default:
		return "Full Arrangement"
	}
}

// Retain the old entry point used by existing scripts/tests. New conversions
// explicitly pass a chosen quality; full is the persisted default.
func createLyreScore(notes []pianoEvent, skipMs int64, speed float64) (lyreScore, error) {
	return createLyreScoreQuality(notes, skipMs, speed, qualityFull)
}

type noteCandidate struct {
	pitch    int
	velocity int
	key      byte
}

func createLyreScoreQuality(notes []pianoEvent, skipMs int64, speed float64, quality string) (lyreScore, error) {
	quality = normalizeLyreQuality(quality)
	if len(notes) == 0 {
		return lyreScore{}, errors.New("no notes available")
	}
	if speed < 0.5 || speed > 2 {
		return lyreScore{}, errors.New("speed must be between 0.5 and 2.0")
	}
	picked := make([]pianoEvent, 0, len(notes))
	for _, n := range notes {
		if n.Ms >= skipMs && n.Pitch >= 0 && n.Pitch <= 127 {
			picked = append(picked, n)
		}
	}
	if len(picked) == 0 {
		return lyreScore{}, errors.New("all notes were skipped; reduce the start offset")
	}
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].Ms < picked[j].Ms })
	shift := pickShift(picked)
	groupMs := int64(18)
	dupMs := int64(32)
	minVel := 0
	maxChord := 6
	if quality == qualityBalanced {
		groupMs = 23
		dupMs = 44
		minVel = 12
		maxChord = 4
	}
	if quality == qualityClean {
		groupMs = 27
		dupMs = 60
		minVel = 16
		maxChord = 2
	}
	// Extremely close repeats from parallel MIDI tracks and transcription
	// noise are collapsed. Human-played repeated notes remain intact.
	first := picked[0].Ms
	groups := make([]lyreEvent, 0, len(picked))
	lastKeyMs := make(map[byte]int64)
	for i := 0; i < len(picked); {
		start := picked[i].Ms
		byKey := make(map[byte]noteCandidate)
		j := i
		for j < len(picked) && picked[j].Ms-start <= groupMs {
			n := picked[j]
			k := nearestLyreKey(n.Pitch + shift)
			if n.Velocity >= minVel || len(byKey) == 0 { // never drop the entire attack purely due to low velocity
				cand := noteCandidate{pitch: n.Pitch + shift, velocity: n.Velocity, key: k}
				old, exists := byKey[k]
				if !exists || cand.velocity > old.velocity || (cand.velocity == old.velocity && cand.pitch > old.pitch) {
					byKey[k] = cand
				}
			}
			j++
		}
		candidates := make([]noteCandidate, 0, len(byKey))
		for _, c := range byKey {
			candidates = append(candidates, c)
		}
		if len(candidates) == 0 {
			i = j
			continue
		}
		// Always retain the most prominent upper voice (melody) and lower
		// voice (bass), then fill remaining voices by velocity. This keeps
		// rich chords while avoiding unplayably dense key clusters.
		melody := 0
		bass := 0
		for a := range candidates {
			if candidates[a].pitch > candidates[melody].pitch {
				melody = a
			}
			if candidates[a].pitch < candidates[bass].pitch {
				bass = a
			}
		}
		chosen := make([]noteCandidate, 0, maxChord)
		used := make(map[byte]bool)
		add := func(c noteCandidate) {
			if !used[c.key] && len(chosen) < maxChord {
				chosen = append(chosen, c)
				used[c.key] = true
			}
		}
		add(candidates[melody])
		if quality != qualityClean || candidates[melody].pitch-candidates[bass].pitch >= 10 {
			add(candidates[bass])
		}
		sort.SliceStable(candidates, func(a, b int) bool {
			va := candidates[a].velocity*10 + candidates[a].pitch/2
			vb := candidates[b].velocity*10 + candidates[b].pitch/2
			if va == vb {
				return candidates[a].pitch > candidates[b].pitch
			}
			return va > vb
		})
		for _, c := range candidates {
			add(c)
		}
		sort.Slice(chosen, func(a, b int) bool { return chosen[a].pitch < chosen[b].pitch })
		keys := []byte{}
		for _, c := range chosen {
			if prev, exists := lastKeyMs[c.key]; exists && start-prev < dupMs {
				continue
			}
			keys = append(keys, c.key)
			lastKeyMs[c.key] = start
		}
		if len(keys) > 0 {
			ms := int64(math.Round(float64(start-first) / speed))
			groups = append(groups, lyreEvent{Ms: ms, Keys: string(keys)})
		}
		i = j
	}
	if len(groups) == 0 {
		return lyreScore{}, errors.New("no playable notes remained after cleanup")
	}
	chords := 0
	for _, ev := range groups {
		if len(ev.Keys) > 1 {
			chords++
		}
	}
	return lyreScore{Quality: quality, Notes: len(picked), Chords: chords, Events: groups, DurationMs: groups[len(groups)-1].Ms, SourceStartMs: skipMs, SemitoneShift: shift, PlaybackSpeed: speed, SourcePiano: append([]pianoEvent(nil), notes...)}, nil
}

func generateLyreAHK(title string, score lyreScore) string {
	var b bytes.Buffer
	b.WriteString("#Requires AutoHotkey v2.0\r\n#SingleInstance Force\r\n; Cazy Lyre v3.4 - generated lyre arrangement. Escape is never mapped.\r\n")
	b.WriteString("; " + strings.ReplaceAll(strings.ReplaceAll(title, "\n", " "), "\r", " ") + "\r\n")
	b.WriteString(fmt.Sprintf("; Quality: %s | Events: %d | Transpose: %+d | Duration: %.1f sec\r\n", qualityDisplay(score.Quality), len(score.Events), score.SemitoneShift, float64(score.DurationMs)/1000))
	b.WriteString(fmt.Sprintf("; Original song start position: %s (%d seconds). Earlier notes omitted.\r\n", formatLyreStartTime(score.SourceStartMs), score.SourceStartMs/1000))
	b.WriteString("SendMode(\"Event\")\r\nSetKeyDelay(0, 0)\r\nPersistent()\r\n")
	b.WriteString("playing := false\r\nnextNote := 1\r\nplayStarted := 0\r\nheld := Map()\r\n")
	b.WriteString("notes := [\r\n")
	for i, event := range score.Events {
		sep := ","
		if i == len(score.Events)-1 {
			sep = ""
		}
		b.WriteString(fmt.Sprintf("    [%d, \"%s\"]%s\r\n", event.Ms, event.Keys, sep))
	}
	b.WriteString("]\r\n\r\nXButton1::StartLyre()  ; Mouse 4: start/restart\r\nXButton2::StopLyre()   ; Mouse 5: stop\r\nF12::ExitApp()         ; Esc remains untouched\r\n\r\n")
	// Timer-driven releases avoid blocking the playback loop with Sleep(28)
	// on every chord. Scheduled timestamps stay anchored to the initial start.
	b.WriteString(`StartLyre() {
    global playing, nextNote, playStarted, held
    StopLyre()
    playing := true
    nextNote := 1
    playStarted := A_TickCount
    held := Map()
    SetTimer(PlayTick, 5)
}
StopLyre() {
    global playing, held
    playing := false
    SetTimer(PlayTick, 0)
    for k, releaseMs in held
        SendEvent("{" k " up}")
    held := Map()
}
PlayTick() {
    global playing, nextNote, playStarted, notes, held
    if !playing
        return
    elapsed := A_TickCount - playStarted
    for k, releaseMs in held.Clone() {
        if releaseMs <= elapsed {
            SendEvent("{" k " up}")
            held.Delete(k)
        }
    }
    sent := 0
    while playing && nextNote <= notes.Length && notes[nextNote][1] <= elapsed && sent < 16 {
        event := notes[nextNote]
        if elapsed - event[1] < 350 {
            for k in StrSplit(event[2]) {
                if held.Has(k) {
                    SendEvent("{" k " up}")
                    held.Delete(k)
                }
                SendEvent("{" k " down}")
                held[k] := elapsed + 23
            }
        }
        nextNote += 1
        sent += 1
    }
    if nextNote > notes.Length && held.Count == 0
        StopLyre()
}
`)
	return b.String()
}
