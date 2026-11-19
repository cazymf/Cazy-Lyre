package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSong = "#Requires AutoHotkey v2.0\r\n; Esc remains untouched\r\nSong := [\r\n    [0, \"q\"],\r\n    [1000, \"av\"],\r\n    [1350, \"mb\"]\r\n]\r\nXButton1::StartLyre()\r\nXButton2::StopLyre()\r\nF12::ExitApp()\r\n"

func TestSongTempoPresets(t *testing.T) {
	for _, tc := range []struct {
		speed         float64
		second, third string
	}{
		{1.25, "[800, \"av\"]", "[1080, \"mb\"]"},
		{1.35, "[741, \"av\"]", "[1000, \"mb\"]"},
		{1.5, "[667, \"av\"]", "[900, \"mb\"]"},
		{2.0, "[500, \"av\"]", "[675, \"mb\"]"},
	} {
		got, rep, err := applySongTempo(testSong, tc.speed)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Events != 3 || rep.OriginalDurationMs != 1350 {
			t.Fatalf("wrong report %v", rep)
		}
		if !strings.Contains(got, tc.second) || !strings.Contains(got, tc.third) {
			t.Errorf("%v: wrong timestamps", tc.speed)
		}
		if !strings.Contains(got, "XButton1::StartLyre()") || !strings.Contains(got, "F12::ExitApp()") || strings.Contains(got, "Esc::") {
			t.Fatal("hotkeys changed")
		}
		if !strings.Contains(got, "\r\n") {
			t.Fatal("CRLF lost")
		}
	}
}
func TestTempoRefusesBrokenInput(t *testing.T) {
	for _, s := range []string{`Sleep(100)`, "notes := [\n[1000, \"a\"]\n[500, \"b\"]\n]", `Esc::ExitApp()` + "\n" + testSong, "notes := [\n[5, \"k\"]\n]", "Song := [\n[0, \"a\"]\n[100, \"%\"]\n]"} {
		if _, _, e := applySongTempo(s, 1.25); e == nil {
			t.Fatalf("must reject invalid input %q", s)
		}
	}
}
func TestTempoRealAHK(t *testing.T) {
	path := filepath.Join("..", "Dream_of_the_Black_Snow_Swan_Lyre_Mouse4_Mouse5.ahk")
	if _, err := os.Stat(path); err != nil {
		t.Skip("fixture not present")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, rep, err := applySongTempo(string(b), 1.35)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Events < 300 || !strings.Contains(got, "XButton1::StartLyre") {
		t.Fatalf("failed to preserve full script: %#v", rep)
	}
}

func TestTempoLegacyEventFormats(t *testing.T) {
	for _, name := range []string{"events", "NoteEvents", "SongEvents"} {
		script := "#Requires AutoHotkey v2.0\n" + name + " := [\n    [200, \"a\"],\n    [400, \"fq\"]\n]\nXButton1::StartLyre()\n"
		faster, rep, err := applySongTempo(script, 1.25)
		if err != nil || rep.Events != 2 {
			t.Fatalf("%s: %v %v", name, rep, err)
		}
		if !strings.Contains(faster, "[160, \"a\"]") || !strings.Contains(faster, "[320, \"fq\"]") {
			t.Fatalf("bad legacy tempo: %s", faster)
		}
	}
}

func TestTempoActualKnownSongs(t *testing.T) {
	songs := []string{
		"I_Will_Survive_Genshin_Lyre_Mouse4_Mouse5.ahk",
		"Dream_of_the_Black_Snow_Swan_Lyre_Mouse4_Mouse5.ahk",
		"Focalors_Dance_Theme_Fast_Start_Mouse4_Mouse5.ahk",
		"Focalors_Dance_Theme_Mouse4_Play_Mouse5_Stop(1).ahk",
	}
	for _, name := range songs {
		data, err := os.ReadFile(filepath.Join("..", name))
		if err != nil {
			t.Skip("fixtures not included")
		}
		result, rep, err := applySongTempo(string(data), 1.50)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if rep.Events < 10 || rep.NewDurationMs >= rep.OriginalDurationMs {
			t.Errorf("unexpected %s report: %+v", name, rep)
		}
		if strings.Contains(result, "Esc::") || !strings.Contains(result, "XButton1") {
			t.Errorf("control issue in %s", name)
		}
	}
}
