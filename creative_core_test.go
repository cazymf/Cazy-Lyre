package main

import (
	"strings"
	"testing"
)

func TestCreativeModeDoesNotEraseNotes(t *testing.T) {
	src := generateLyreAHK("Test", lyreScore{Quality: qualityFull, Events: []lyreEvent{{0, "a"}, {300, "dh"}, {700, "q"}, {900, "g"}, {1200, "sj"}}, DurationMs: 1200})
	s, err := parseAHKLyreScore(src)
	if err != nil || len(s.Events) != 5 {
		t.Fatalf("read music: %v, %d", err, len(s.Events))
	}
	chunk := s.Events
	good := `{"events":[{"ms":0,"keys":"a"},{"ms":350,"keys":"dh"},{"ms":720,"keys":"q"},{"ms":980,"keys":"g"},{"ms":1200,"keys":"sj"}]}`
	edits, err := parseCreativeEvents(good, chunk, 1200)
	if err != nil || len(edits) != 5 || edits[1].Ms != 350 {
		t.Fatalf("creative note edits: %v, %#v", err, edits)
	}
	s.Events = edits
	out := generateLyreAHK("New", s)
	if strings.Contains(out, "Esc::") || !strings.Contains(out, "XButton1::") || !strings.Contains(out, "XButton2::") || !strings.Contains(out, "F12::") {
		t.Fatal("broken controls")
	}
	if !strings.Contains(out, `[350, "dh"]`) {
		t.Fatal("missing melody corrections")
	}
	if err := validateNonemptyAHK(out); err != nil {
		t.Fatal(err)
	}
}
func TestCreativeRejectsEmptyOrWrongNotes(t *testing.T) {
	original := []lyreEvent{{0, "a"}, {200, "s"}, {400, "d"}, {600, "f"}, {800, "g"}, {1000, "h"}, {1200, "j"}, {1400, "q"}}
	bad := []string{`{"events":[]}`, `{"events":[{"ms":20,"keys":"!"}]}`, `{"events":[{"ms":0,"keys":"a"},{"ms":100,"keys":"s"},{"ms":200,"keys":"d"},{"ms":100000,"keys":"f"}]}`, `oops`}
	for _, text := range bad {
		if _, err := parseCreativeEvents(text, original, 1400); err == nil {
			t.Errorf("accepted broken AI output: %s", text)
		}
	}
}
func TestCreativeSourceMIDIReference(t *testing.T) {
	bundle := lyreSourceBundle{StartMs: 10000, PlaybackSpeed: 1.0, PianoNotes: []pianoEvent{{Ms: 10050, Pitch: 61}, {Ms: 10400, Pitch: 64}, {Ms: 5000, Pitch: 1}}}
	c := []lyreEvent{{0, "a"}, {400, "s"}}
	ref := originalPitchReference(bundle, c)
	if len(ref) != 2 || ref[0].Keys != "61" || ref[1].Ms != 400 {
		t.Fatalf("invalid MIDI pitch reference: %#v", ref)
	}
}
func TestCreativeChunkBoundaries(t *testing.T) {
	ev := make([]lyreEvent, 145)
	for i := range ev {
		ev[i] = lyreEvent{Ms: int64(i * 150), Keys: "a"}
	}
	batches := chunkCreativeScore(ev, 70)
	if len(batches) != 3 || len(batches[0]) != 70 || len(batches[2]) != 5 {
		t.Fatalf("chunk sizes: %d", len(batches))
	}
}

func TestCreativeChordFormattingAndRecovery(t *testing.T) {
	cases := map[string]string{
		"A + S":   "as",
		"[q,w,e]": "qwe",
		"a|s|a":   "as",
		"  Q W  ": "qw",
	}
	for raw, want := range cases {
		got, err := normalizeCreativeChord(raw)
		if err != nil || got != want {
			t.Errorf("normalize %q => %q (%v), want %q", raw, got, err, want)
		}
	}
	for _, bad := range []string{"", "C#4", "N/A", "?", "a1"} {
		if _, err := normalizeCreativeChord(bad); err == nil {
			t.Errorf("unexpectedly accepted invalid chord %q", bad)
		}
	}
}
func TestCreativeRepairEmptyKeyFromOriginal(t *testing.T) {
	original := []lyreEvent{{10000, "q"}, {10300, "w"}, {10600, "e"}, {10900, "r"}, {11200, "t"}, {11500, "y"}}
	response := `{"events":[{"ms":0,"keys":"q"},{"ms":300,"keys":" "},{"ms":600,"keys":"e"},{"ms":900,"keys":"r"},{"ms":1200,"keys":"t"},{"ms":1500,"keys":"y"}]}`
	edits, repairs, keptOriginal, reason := chooseCreativeSection(response, original, 1500)
	if keptOriginal || reason != "" || repairs != 1 || len(edits) != len(original) || edits[1].Keys != "w" {
		t.Fatalf("single blank key should be repaired: %#v repairs=%d kept=%v reason=%q", edits, repairs, keptOriginal, reason)
	}
}
func TestCreativeKeepsOriginalOnBadResponse(t *testing.T) {
	original := []lyreEvent{{10000, "q"}, {10300, "w"}, {10600, "e"}, {10900, "r"}, {11200, "t"}, {11500, "y"}}
	edits, repairs, keptOriginal, reason := chooseCreativeSection(`{"events":[{"ms":0,"keys":"????"}]}`, original, 1500)
	if !keptOriginal || repairs != 0 || reason == "" || len(edits) != len(original) || edits[0].Ms != 0 || edits[0].Keys != "q" || edits[5].Ms != 1500 {
		t.Fatalf("fallback must restore complete original section: %#v repaired=%d kept=%v err=%q", edits, repairs, keptOriginal, reason)
	}
}
