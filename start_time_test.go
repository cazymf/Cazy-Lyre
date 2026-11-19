package main

import (
	"strings"
	"testing"
)

func TestParseLyreStartTime(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  int64
	}{
		{"0", 0}, {"0:00", 0}, {"1:40", 100000}, {"01:40", 100000},
		{"1.40", 100000}, {"100", 100000}, {"1:40.5", 100500},
		{"1.5", 1500}, {"01:02:03", 3723000}, {"120:00", 7200000},
	} {
		got, err := parseLyreStartTime(tt.input)
		if err != nil || got != tt.want {
			t.Errorf("parse %q: got %d, %v, want %d", tt.input, got, err, tt.want)
		}
	}
	for _, input := range []string{"", "-1", "1:60", "1.99", "121:00", "2:00:01", "x", "1:2:3:4"} {
		if _, err := parseLyreStartTime(input); err == nil {
			t.Errorf("expected error for %q", input)
		}
	}
}
func TestStartTimeNudges(t *testing.T) {
	steps := []struct {
		start string
		nudge int64
		want  string
	}{
		{"1:40", 1, "1:41"}, {"1.40", 10, "1:50"}, {"1:55", 10, "2:05"},
		{"2:00", -10, "1:50"}, {"0:05", -10, "0:00"}, {"120:00", 60, "120:00"},
		{"0:00", 60, "1:00"},
	}
	for _, tc := range steps {
		got, err := nudgeLyreStartTime(tc.start, tc.nudge)
		if err != nil || got != tc.want {
			t.Errorf("nudge %s %+d = %q, %v; want %s", tc.start, tc.nudge, got, err, tc.want)
		}
	}
}

func TestSkipReallyStartsAtChosenTime(t *testing.T) {
	// The first 100 seconds have a distinct pitch; after 1:40 the music changes.
	// Skipping must delete every earlier note, not merely change the label.
	notes := []pianoEvent{
		{Pitch: 48, Velocity: 90, Ms: 0},
		{Pitch: 48, Velocity: 90, Ms: 50000},
		{Pitch: 48, Velocity: 90, Ms: 99000},
		{Pitch: 79, Velocity: 90, Ms: 100000},
		{Pitch: 81, Velocity: 90, Ms: 101000},
	}
	off, err := parseLyreStartTime("1.40")
	if err != nil {
		t.Fatal(err)
	}
	got, err := createLyreScoreQuality(notes, off, 1, qualityFull)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceStartMs != 100000 || got.Notes != 2 || len(got.Events) != 2 {
		t.Fatalf("should retain only 2 notes after 1:40: %+v", got)
	}
	if got.Events[0].Ms != 0 || got.Events[1].Ms != 1000 {
		t.Fatalf("wrong relative timing after intro skip: %+v", got.Events)
	}
	code := generateLyreAHK("After intro", got)
	for _, required := range []string{"Original song start position: 1:40 (100 seconds)", "XButton1::StartLyre()", "XButton2::StopLyre()"} {
		if !strings.Contains(code, required) {
			t.Errorf("missing %q", required)
		}
	}
	if strings.Contains(code, "Esc::") || strings.Contains(code, "Escape::") {
		t.Fatal("Never bind Esc")
	}
}
