//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var tempoPresets = []float64{1.00, 1.10, 1.25, 1.35, 1.50, 1.75, 2.00}

func openTempoLab() {
	i := selectedIndex()
	if i < 0 {
		status("Select an existing .ahk song in Macro Library, then click SPEED UP.")
		return
	}
	if !strings.EqualFold(filepath.Ext(app.items[i].Path), ".ahk") {
		status("Tempo Lab needs an .ahk song, not an executable. Select a lyre AHK script.")
		return
	}
	app.tempoSpeed = 1.25
	switchPage("tempo")
	status("Tempo Lab: choose 1.25x, 1.35x or 1.50x, then CREATE FASTER COPY. Original stays untouched.")
}

func pickTempoSpeed(value float64) {
	if !validTempoSpeed(value) {
		return
	}
	app.tempoSpeed = value
	status("Tempo Lab set to " + tempoSpeedLabel(value) + ". Click CREATE FASTER COPY when ready.")
}

func createTempoVariant() {
	i := selectedIndex()
	if i < 0 || !strings.EqualFold(filepath.Ext(app.items[i].Path), ".ahk") {
		status("Select an AHK song in Macro Library first.")
		return
	}
	original := app.items[i]
	b, err := os.ReadFile(original.Path)
	if err != nil {
		status("Could not read selected song: " + err.Error())
		return
	}
	speed := app.tempoSpeed
	converted, report, err := applySongTempo(string(b), speed)
	if err != nil {
		msgBox("Tempo Lab — Unsupported Script", err.Error()+"\n\nThe original song was not changed.", MB_OK|MB_ICONERROR)
		status("Speed adjustment rejected: " + err.Error())
		return
	}
	dir := filepath.Join(filepath.Dir(appDataPath()), "Tempo Exports")
	if err = os.MkdirAll(dir, 0755); err != nil {
		status("Could not create tempo output folder: " + err.Error())
		return
	}
	basename := strings.TrimSuffix(filepath.Base(original.Path), filepath.Ext(original.Path))
	// Writes a new song; never modifies the source nor overwrites a prior variant.
	outName := basename + "_" + tempoVariantSuffix(speed)
	var outPath string
	var file *os.File
	for attempt := 0; attempt < 1000; attempt++ {
		name := outName
		if attempt > 0 {
			name = fmt.Sprintf("%s_%d", outName, attempt+1)
		}
		outPath = filepath.Join(dir, name+".ahk")
		file, err = os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if os.IsExist(err) {
			continue
		}
		break
	}
	if err != nil {
		status("Could not save faster version: " + err.Error())
		return
	}
	_, err = file.WriteString(converted)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(outPath)
		status("Could not write faster version: " + err.Error())
		return
	}
	app.items = append(app.items, macro{Name: displayName(outPath), Path: outPath, Added: time.Now().Unix()})
	app.selected = outPath
	save()
	switchPage("library")
	status(fmt.Sprintf("CREATED %s: %d events; %.1fs -> %.1fs. Original untouched. Select LAUNCH to test.", tempoSpeedLabel(speed), report.Events, float64(report.OriginalDurationMs)/1000, float64(report.NewDurationMs)/1000))
}

func paintTempo(hdc uintptr, w, h int) {
	i := selectedIndex()
	if i < 0 || !strings.EqualFold(filepath.Ext(app.items[i].Path), ".ahk") {
		textDraw(hdc, "Select an AHK song in the library first.", 304, 314, w-390, 38, palette.white, app.headingFont, DT_LEFT)
		button(hdc, "tempo-back", "BACK TO LIBRARY", 304, 387, 220, 40, 1)
		return
	}
	rounded(hdc, 283, 287, w-315, 335, palette.panel, palette.border, 18)
	textDraw(hdc, "SELECTED AHK SONG", 304, 302, w-385, 24, palette.accent2, app.microFont, DT_LEFT)
	textDraw(hdc, byteshort(app.items[i].Name, 66), 304, 331, w-385, 35, palette.white, app.headingFont, DT_LEFT)
	textDraw(hdc, byteshort(app.items[i].Path, 100), 304, 372, w-388, 23, palette.muted, app.subFont, DT_LEFT)
	textDraw(hdc, "PLAYBACK SPEED — choose a multiplier", 304, 414, w-380, 27, palette.white, app.itemFont, DT_LEFT)
	// Three strongly requested choices remain visible and prominent.
	picks := []struct {
		speed float64
		label string
	}{
		{1.0, "1.00x"}, {1.10, "1.10x"}, {1.25, "1.25x"}, {1.35, "1.35x"}, {1.50, "1.50x"}, {1.75, "1.75x"}, {2.0, "2.00x"},
	}
	x, y := 304, 447
	for _, p := range picks {
		style := 0
		if app.tempoSpeed == p.speed {
			style = 1
		}
		button(hdc, "tempo-pick:"+p.label, p.label, x, y, 101, 38, style)
		x += 111
		if x+101 > w-55 {
			x = 304
			y += 46
		}
	}
	textDraw(hdc, "Only timed lyre note arrays are supported. Chords + notes stay in sync.", 304, 543, w-382, 24, palette.muted, app.subFont, DT_LEFT)
	button(hdc, "tempo-create", "CREATE FASTER COPY", 304, 572, 244, 43, 2)
	button(hdc, "tempo-back", "BACK TO LIBRARY", 563, 572, 212, 43, 0)
	textDraw(hdc, "No API credits. Never overwrites the original. Mouse 4 / Mouse 5 / F12 unchanged; Esc untouched.", 304, h-83, w-366, 28, palette.green, app.subFont, DT_LEFT)
}
