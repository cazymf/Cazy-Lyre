package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const timelineFixture = "#Requires AutoHotkey v2.0\r\nnotes := [\r\n    [0, \"Q\"],\r\n    [500, \"W\"],\r\n    [1000, \"AC\"],\r\n    [1500, \"E\"],\r\n    [1500, \"FB\"],\r\n    [2000, \"R\"],\r\n    [2500, \"T\"],\r\n    [3000, \"Y\"]\r\n]\r\nXButton1::Start()\r\nXButton2::Stop()\r\nF12::ExitApp()\r\n"

func TestTimelineTrimAndCuts(t *testing.T) {
	got, rep, err := cutLyreTimedSong(timelineFixture, 500, 3000, []lyreTimeCut{{1000, 2000}, {2500, 2700}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.OriginalEvents != 8 || rep.SavedEvents != 3 || rep.RemovedEvents != 5 || rep.RemovedDurationMs != 1200 || rep.NewDurationMs != 1300 {
		t.Fatalf("report: %+v", rep)
	}
	for _, n := range []string{"[0, \"W\"]", "[500, \"R\"]", "[1300, \"Y\"]"} {
		if !strings.Contains(got, n) {
			t.Fatalf("expected %s", n)
		}
	}
	if strings.Contains(got, "[1000, \"AC\"]") || strings.Contains(got, "[1500, \"FB\"]") {
		t.Fatal("cut event survived")
	}
	if !strings.Contains(got, "XButton1::Start()") || !strings.Contains(got, "XButton2::Stop()") || !strings.Contains(got, "F12::ExitApp()") {
		t.Fatal("hotkeys changed")
	}
	if !strings.Contains(got, "\r\n") {
		t.Fatal("CRLF not preserved")
	}
	if _, err := parseLyreTimedSong(got); err != nil {
		t.Fatalf("result no longer parses: %v", err)
	}
	if _, rep2, err := applySongTempo(got, 1.5); err != nil || rep2.Events != rep.SavedEvents {
		t.Fatalf("Tempo Lab incompatible: %v %+v", err, rep2)
	}
}

func TestTimelineMultipleNotesAtSameTime(t *testing.T) {
	got, rep, err := cutLyreTimedSong(timelineFixture, 1500, 3000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "[0, \"E\"]") || !strings.Contains(got, "[0, \"FB\"]") || rep.SavedEvents != 5 {
		t.Fatal("simultaneous notes/chords lost")
	}
}

func TestTimelineRejectsInvalidRanges(t *testing.T) {
	for _, tc := range []struct {
		start, end int64
		cuts       []lyreTimeCut
	}{
		{0, 0, nil}, {1000, 500, nil}, {0, 3200, nil}, {0, 3000, []lyreTimeCut{{1900, 2300}, {2200, 2400}}},
		{0, 3000, []lyreTimeCut{{400, 400}}}, {500, 3000, []lyreTimeCut{{0, 200}}},
		{0, 3000, []lyreTimeCut{{0, 3000}}},
	} {
		if _, _, err := cutLyreTimedSong(timelineFixture, tc.start, tc.end, tc.cuts); err == nil {
			t.Fatalf("expected rejection: %+v", tc)
		}
	}
}

func TestTimelineRealSyncedSong(t *testing.T) {
	data := strings.TrimSpace(`#Requires AutoHotkey v2.0
notes := [
 [0, "A"],
 [316, "WQ"],
 [900, "V"],
 [1300, "AS"],
 [4000, "B"]
]
XButton1::Start()
XButton2::Stop()
F12::ExitApp()`)
	got, rep, err := cutLyreTimedSong(data, 250, 4000, []lyreTimeCut{{1000, 3000}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.SavedEvents != 3 {
		t.Fatal(rep)
	}
	for _, at := range []int{66, 650, 1750} {
		if !strings.Contains(got, fmt.Sprintf("[%d,", at)) {
			t.Fatalf("expected timestamp %d", at)
		}
	}
}

// Regression test against the exact 841-event synced Katy Perry macro.
func TestTimelineActualKatyMacro(t *testing.T) {
	b, err := os.ReadFile("/tmp/cazy_sample_katy.ahk")
	if err != nil {
		t.Skip("Katy fixture not present")
	}
	events, err := parseLyreTimedSong(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 841 {
		t.Fatalf("expected 841 events, got %d", len(events))
	}
	got, rep, err := cutLyreTimedSong(string(b), 10000, events[len(events)-1].TimeMs, []lyreTimeCut{{30000, 40000}, {80000, 90000}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.SavedEvents <= 0 || rep.SavedEvents >= 841 {
		t.Fatalf("unexpected saved event count %+v", rep)
	}
	if _, err := parseLyreTimedSong(got); err != nil {
		t.Fatalf("failed parsing edited cover: %v", err)
	}
	if _, _, err := applySongTempo(got, 1.25); err != nil {
		t.Fatalf("Tempo Lab cannot use edited Katy cover: %v", err)
	}
}
