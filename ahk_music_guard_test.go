package main

import (
	"strings"
	"testing"
)

func TestMusicArrayProtectionAndRestore(t *testing.T) {
	src := "#Requires AutoHotkey v2.0\nSong := [\n [0, \"n\"],\n [133, \"d\"],\n [267, \"hq\"]\n]\nXButton1::PlaySong()\nPlaySong() {\n SendEvent(\"n\")\n}\n"
	masked, blocks, count, err := protectLyreNotes(src)
	if err != nil || len(blocks) != 1 || count != 3 {
		t.Fatalf("protect failed: %v, %d blocks, %d events", err, len(blocks), count)
	}
	if strings.Contains(masked, "[133") {
		t.Fatal("note data leaked into AI prompt")
	}
	fixed := strings.Replace(masked, "SendEvent(\"n\")", "SendEvent(\"q\")", 1)
	restored, err := restoreLyreNotes(fixed, blocks, count)
	if err != nil || !strings.Contains(restored, "[133, \"d\"]") {
		t.Fatalf("restore failed: %v", err)
	}
	if err := validateNonemptyAHK(restored); err != nil {
		t.Fatal(err)
	}
}
func TestRejectDeletedNoteData(t *testing.T) {
	src := "Song := [[0, \"a\"], [150, \"b\"]]\nXButton1::SendEvent(\"a\")"
	_, blocks, count, _ := protectLyreNotes(src)
	if _, err := restoreLyreNotes("XButton1::SendEvent(\"a\")", blocks, count); err == nil {
		t.Fatal("missing notes accepted")
	}
}
func TestLocalEventsPreserved(t *testing.T) {
	src := "Play() {\n  events := [\n [0, \"y\"], [400, \"q\"]\n ]\n for e in events\n  SendEvent(e[2])\n}\nXButton1::Play()"
	masked, blocks, n, err := protectLyreNotes(src)
	if err != nil || len(blocks) != 1 || n != 2 {
		t.Fatalf("incorrect local mask: %v %v %d", err, blocks, n)
	}
	restored, err := restoreLyreNotes(masked, blocks, n)
	if err != nil || restored != src {
		t.Fatalf("roundtrip changed source: %v", err)
	}
}
func TestGeneratedScriptMustHaveSend(t *testing.T) {
	if err := validateNonemptyAHK("#Requires AutoHotkey v2.0\n#SingleInstance Force\nXButton1::{\n   MsgBox(\"hello\")\n}\n; no keys\n"); err == nil {
		t.Fatal("silent macro accepted")
	}
}

func TestRejectClippedAHK(t *testing.T) {
	clipped := "#Requires AutoHotkey v2.0\nXButton1::{\n SendEvent(\"f\")\n"
	if err := validateNonemptyAHK(clipped); err == nil {
		t.Fatal("clipped AI output was accepted")
	}
}
