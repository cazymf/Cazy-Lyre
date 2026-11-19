package main

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func vlq(n uint64) []byte {
	var out []byte
	out = append(out, byte(n&127))
	n >>= 7
	for n > 0 {
		out = append([]byte{byte(n&127) | 0x80}, out...)
		n >>= 7
	}
	return out
}
func event(out *bytes.Buffer, delta uint64, values ...byte) { out.Write(vlq(delta)); out.Write(values) }
func fixture() []byte {
	var track bytes.Buffer
	// A major A4 C#5 E5 chord to validate chromatic transposition.
	event(&track, 0, 0xFF, 0x51, 3, 0x07, 0xA1, 0x20)
	event(&track, 0, 0x90, 69, 100)
	event(&track, 0, 0x90, 73, 100)
	event(&track, 0, 0x90, 76, 100)
	event(&track, 480, 0x80, 69, 0)
	event(&track, 0, 0x80, 73, 0)
	event(&track, 0, 0x80, 76, 0)
	event(&track, 0, 0xFF, 0x51, 3, 0x06, 0x1A, 0x80) // tempo change
	event(&track, 480, 0x90, 69, 100)
	event(&track, 0, 0xFF, 0x2f, 0)
	var m bytes.Buffer
	m.WriteString("MThd")
	binary.Write(&m, binary.BigEndian, uint32(6))
	binary.Write(&m, binary.BigEndian, uint16(0))
	binary.Write(&m, binary.BigEndian, uint16(1))
	binary.Write(&m, binary.BigEndian, uint16(480))
	m.WriteString("MTrk")
	binary.Write(&m, binary.BigEndian, uint32(track.Len()))
	m.Write(track.Bytes())
	return m.Bytes()
}
func TestMIDIConvert(t *testing.T) {
	notes, err := parseMIDI(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 4 {
		t.Fatalf("4 notes expected, got %d", len(notes))
	}
	if notes[3].Ms != 900 {
		t.Fatalf("expected second chord note at 900ms, got %d", notes[3].Ms)
	}
	score, err := createLyreScore(notes, 0, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if len(score.Events) != 2 {
		t.Fatalf("expected chord and one note, got %+v", score.Events)
	}
	for _, ev := range score.Events {
		if ev.Keys == "" {
			t.Error("empty lyre event")
		}
	}
	code := generateLyreAHK("Test", score)
	for _, expect := range []string{"XButton1::StartLyre()", "XButton2::StopLyre()", "F12::ExitApp()", "[900,"} {
		if !strings.Contains(code, expect) {
			t.Errorf("missing %q", expect)
		}
	}
	if strings.Contains(code, "Esc::") || strings.Contains(code, "Escape::") {
		t.Error("Escape hotkey must never exist")
	}
}
func TestSkipAndSpeed(t *testing.T) {
	notes, e := parseMIDI(fixture())
	if e != nil {
		t.Fatal(e)
	}
	score, e := createLyreScore(notes, 500, 2.0)
	if e != nil {
		t.Fatal(e)
	}
	if len(score.Events) != 1 || score.Events[0].Ms != 0 {
		t.Fatalf("unexpected %+v", score.Events)
	}
}

func TestLyreQualityModes(t *testing.T) {
	var notes []pianoEvent
	for _, p := range []int{48, 50, 52, 53, 55, 57, 59, 60, 62, 64, 65, 67} {
		notes = append(notes, pianoEvent{Pitch: p, Velocity: 100, Ms: 0})
	}
	for _, p := range []int{72, 74, 76, 77, 79} {
		notes = append(notes, pianoEvent{Pitch: p, Velocity: 110, Ms: 450})
	}
	full, err := createLyreScoreQuality(notes, 0, 1, qualityFull)
	if err != nil {
		t.Fatal(err)
	}
	balanced, err := createLyreScoreQuality(notes, 0, 1, qualityBalanced)
	if err != nil {
		t.Fatal(err)
	}
	clean, err := createLyreScoreQuality(notes, 0, 1, qualityClean)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Events) != 2 || len(balanced.Events) != 2 || len(clean.Events) != 2 {
		t.Fatalf("unexpected event counts: full %d balanced %d clean %d", len(full.Events), len(balanced.Events), len(clean.Events))
	}
	if len(full.Events[0].Keys) != 6 {
		t.Fatalf("full should retain 6 parts, got %q", full.Events[0].Keys)
	}
	if len(balanced.Events[0].Keys) > 4 {
		t.Fatalf("balanced chord too dense: %q", balanced.Events[0].Keys)
	}
	if len(clean.Events[0].Keys) > 2 {
		t.Fatalf("clean chord too dense: %q", clean.Events[0].Keys)
	}
	if normalizeLyreQuality("") != qualityFull {
		t.Fatal("default must be Full Arrangement")
	}
	code := generateLyreAHK("Quality Test", full)
	for _, hotkey := range []string{"XButton1::StartLyre()", "XButton2::StopLyre()", "F12::ExitApp()"} {
		if !strings.Contains(code, hotkey) {
			t.Errorf("missing %s", hotkey)
		}
	}
	if strings.Contains(code, "Esc::") || strings.Contains(code, "Escape::") {
		t.Fatal("Esc must not be intercepted")
	}
	if strings.Contains(code, "Sleep(28)") {
		t.Fatal("AHK timer should not block 28ms on each chord")
	}
}
func TestCloseRepeatedNotesCleanup(t *testing.T) {
	notes := []pianoEvent{{Pitch: 72, Velocity: 90, Ms: 100}, {Pitch: 72, Velocity: 85, Ms: 112}, {Pitch: 72, Velocity: 90, Ms: 260}, {Pitch: 76, Velocity: 90, Ms: 260}}
	score, err := createLyreScoreQuality(notes, 0, 1, qualityFull)
	if err != nil {
		t.Fatal(err)
	}
	if len(score.Events) != 2 {
		t.Fatalf("expect collapsed duplicate at 112ms, got %#v", score.Events)
	}
	if score.Events[1].Ms != 160 {
		t.Fatalf("expected relative timing 160ms, got %d", score.Events[1].Ms)
	}
}
