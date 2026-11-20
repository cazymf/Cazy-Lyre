//go:build windows

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Timeline Lab keeps the loaded song in memory and always writes an independent
// export. There is deliberately no in-place overwrite action.
var timeline struct {
	Source, Path, Title          string
	Events                       []lyreTimedEvent
	Duration, Cursor, Start, End int64
	CutIn, CutOut                int64
	HaveCutIn, HaveCutOut        bool
	Cuts                         []lyreTimeCut
}

func openTimelineLab() {
	i := selectedIndex()
	if i < 0 || !strings.EqualFold(filepath.Ext(app.items[i].Path), ".ahk") {
		status("Select an .ahk song in Macro Library, then click EDIT TIMELINE.")
		return
	}
	item := app.items[i]
	b, err := os.ReadFile(item.Path)
	if err != nil {
		msgBox("Timeline Lab", err.Error(), MB_OK|MB_ICONERROR)
		return
	}
	events, err := parseLyreTimedSong(string(b))
	if err != nil {
		msgBox("Timeline Lab — Unsupported Script", err.Error()+"\n\nThe original macro was not changed.", MB_OK|MB_ICONERROR)
		return
	}
	duration := events[len(events)-1].TimeMs
	if duration <= 0 {
		msgBox("Timeline Lab", "This song has no measurable timeline.", MB_OK|MB_ICONERROR)
		return
	}
	timeline.Source = string(b)
	timeline.Path = item.Path
	timeline.Title = item.Name
	if timeline.Title == "" {
		timeline.Title = displayName(item.Path)
	}
	timeline.Events = events
	timeline.Duration = duration
	timeline.Start = 0
	timeline.End = duration
	timeline.Cursor = 0
	timeline.Cuts = nil
	timeline.HaveCutIn = false
	timeline.HaveCutOut = false
	switchPage("timeline")
	status("Timeline Lab: click or drag on the timeline, place START/END, or mark cuts. Save as a new macro.")
}

func lyreTimeText(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	minutes := ms / 60000
	seconds := ms % 60000 / 1000
	hundredths := (ms % 1000) / 10
	return fmt.Sprintf("%02d:%02d.%02d", minutes, seconds, hundredths)
}
func maxTime(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func minTime(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func clampTimeline(ms int64) int64 { return maxTime(0, minTime(ms, timeline.Duration)) }

func timelineScrub(x int) {
	width := app.w - 385
	if width <= 0 || timeline.Duration <= 0 {
		return
	}
	pos := maxInt(0, minInt(width, x-315))
	timeline.Cursor = int64(math.Round(float64(pos) * float64(timeline.Duration) / float64(width)))
	redraw()
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func timelineNudge(delta int64) {
	timeline.Cursor = clampTimeline(timeline.Cursor + delta)
	redraw()
}
func timelineCutsInRange(start, end int64) []lyreTimeCut {
	cuts := make([]lyreTimeCut, 0, len(timeline.Cuts))
	for _, c := range timeline.Cuts {
		lo, hi := maxTime(c.StartMs, start), minTime(c.EndMs, end)
		if hi > lo {
			cuts = append(cuts, lyreTimeCut{lo, hi})
		}
	}
	return cuts
}
func timelineSetStart() {
	if timeline.Cursor >= timeline.End {
		status("START must be before END.")
		return
	}
	timeline.Start = timeline.Cursor
	timeline.Cuts = timelineCutsInRange(timeline.Start, timeline.End)
	redraw()
}
func timelineSetEnd() {
	if timeline.Cursor <= timeline.Start {
		status("END must be later than START.")
		return
	}
	timeline.End = timeline.Cursor
	timeline.Cuts = timelineCutsInRange(timeline.Start, timeline.End)
	redraw()
}
func timelineMark(which string) {
	if timeline.Cursor < timeline.Start || timeline.Cursor > timeline.End {
		status("Place cut markers within START and END.")
		return
	}
	if which == "in" {
		timeline.CutIn = timeline.Cursor
		timeline.HaveCutIn = true
	} else {
		timeline.CutOut = timeline.Cursor
		timeline.HaveCutOut = true
	}
	redraw()
}
func timelineAddCut() {
	if !timeline.HaveCutIn || !timeline.HaveCutOut {
		status("Mark CUT IN and CUT OUT before adding a cut.")
		return
	}
	lo, hi := timeline.CutIn, timeline.CutOut
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < timeline.Start || hi > timeline.End || hi-lo < 1 {
		status("Cut must have nonzero length inside START/END.")
		return
	}
	cuts := append(append([]lyreTimeCut{}, timeline.Cuts...), lyreTimeCut{lo, hi})
	sort.Slice(cuts, func(i, j int) bool { return cuts[i].StartMs < cuts[j].StartMs })
	merged := make([]lyreTimeCut, 0, len(cuts))
	for _, c := range cuts {
		if len(merged) == 0 || c.StartMs > merged[len(merged)-1].EndMs {
			merged = append(merged, c)
		} else if c.EndMs > merged[len(merged)-1].EndMs {
			merged[len(merged)-1].EndMs = c.EndMs
		}
	}
	if len(merged) > 200 {
		status("Too many cuts; max 200 merged cut regions.")
		return
	}
	timeline.Cuts = merged
	timeline.HaveCutIn, timeline.HaveCutOut = false, false
	status(fmt.Sprintf("Cut added. %d region(s) will be removed when you save.", len(merged)))
}
func timelineUndoCut() {
	if len(timeline.Cuts) == 0 {
		return
	}
	timeline.Cuts = timeline.Cuts[:len(timeline.Cuts)-1]
	redraw()
}
func timelineReset() {
	timeline.Start, timeline.End, timeline.Cursor = 0, timeline.Duration, 0
	timeline.Cuts = nil
	timeline.HaveCutIn, timeline.HaveCutOut = false, false
	redraw()
}
func timelinePredictedDuration() int64 {
	remaining := timeline.End - timeline.Start
	for _, c := range timeline.Cuts {
		remaining -= c.EndMs - c.StartMs
	}
	return maxTime(0, remaining)
}

func saveTimelineVariant() {
	if timeline.Path == "" {
		status("Open a song in Timeline Lab first.")
		return
	}
	result, report, err := cutLyreTimedSong(timeline.Source, timeline.Start, timeline.End, timeline.Cuts)
	if err != nil {
		msgBox("Timeline Lab — Cannot Save", err.Error()+"\n\nThe original macro was not modified.", MB_OK|MB_ICONERROR)
		status("Cannot save timeline edit: " + err.Error())
		return
	}
	dir := filepath.Join(filepath.Dir(appDataPath()), "Timeline Exports")
	if err = os.MkdirAll(dir, 0755); err != nil {
		status("Could not create timeline exports folder: " + err.Error())
		return
	}
	basename := strings.TrimSuffix(filepath.Base(timeline.Path), filepath.Ext(timeline.Path))
	var path string
	var f *os.File
	for attempt := 0; attempt < 1000; attempt++ {
		name := basename + "_Timeline_Edit"
		if attempt > 0 {
			name = fmt.Sprintf("%s_%d", name, attempt+1)
		}
		path = filepath.Join(dir, name+".ahk")
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if os.IsExist(err) {
			continue
		}
		break
	}
	if err != nil {
		status("Could not create edited macro: " + err.Error())
		return
	}
	_, err = f.WriteString(result)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		status("Could not write edited macro: " + err.Error())
		return
	}
	app.items = append(app.items, macro{Name: displayName(path), Path: path, Added: time.Now().Unix()})
	app.selected = path
	save()
	switchPage("library")
	status(fmt.Sprintf("TIMELINE SAVED: %d of %d events; %s -> %s. Original untouched; Tempo Lab ready.", report.SavedEvents, report.OriginalEvents, lyreTimeText(report.OriginalDurationMs), lyreTimeText(report.NewDurationMs)))
}

func clickTimelineAction(id string) bool {
	if !strings.HasPrefix(id, "tl-") && !strings.HasPrefix(id, "tl+") {
		return false
	}
	switch id {
	case "tl-open":
		openTimelineLab()
	case "tl-back":
		switchPage("library")
	case "tl-start":
		timelineSetStart()
	case "tl-end":
		timelineSetEnd()
	case "tl-in":
		timelineMark("in")
	case "tl-out":
		timelineMark("out")
	case "tl-add":
		timelineAddCut()
	case "tl-undo":
		timelineUndoCut()
	case "tl-clear":
		timeline.Cuts = nil
		timeline.HaveCutIn, timeline.HaveCutOut = false, false
		redraw()
	case "tl-reset":
		timelineReset()
	case "tl-save":
		saveTimelineVariant()
	case "tl-10":
		timelineNudge(-10000)
	case "tl-1":
		timelineNudge(-1000)
	case "tl-01":
		timelineNudge(-100)
	case "tl+01":
		timelineNudge(100)
	case "tl+1":
		timelineNudge(1000)
	case "tl+10":
		timelineNudge(10000)
	}
	return true
}

func paintTimeline(hdc uintptr, w, h int) {
	if timeline.Duration <= 0 {
		textDraw(hdc, "Select an AHK macro to edit.", 306, 320, w-350, 32, palette.muted, app.itemFont, DT_LEFT)
		button(hdc, "tl-back", "BACK TO LIBRARY", 309, 410, 220, 41, 0)
		return
	}
	rounded(hdc, 283, 287, w-315, 360, palette.panel, palette.border, 17)
	textDraw(hdc, "EDITING A COPY OF", 310, 300, 400, 24, palette.accent2, app.microFont, DT_LEFT)
	textDraw(hdc, byteshort(timeline.Title, 65), 309, 327, w-365, 32, palette.white, app.headingFont, DT_LEFT)
	textDraw(hdc, fmt.Sprintf("SONG %s    START %s    END %s    OUTPUT ~%s    CUTS %d",
		lyreTimeText(timeline.Duration), lyreTimeText(timeline.Start), lyreTimeText(timeline.End),
		lyreTimeText(timelinePredictedDuration()), len(timeline.Cuts)), 310, 359, w-345, 22, palette.muted, app.microFont, DT_LEFT)

	// Each bar represents density of actual note events, rather than a rendered
	// waveform that might imply audio data is present in an AHK macro.
	trackX, trackY, trackW, trackH := 315, 389, w-385, 53
	rounded(hdc, trackX-3, trackY-4, trackW+6, trackH+8, palette.edit, palette.border, 8)
	bins := minInt(trackW/3, 220)
	if bins < 1 {
		bins = 1
	}
	density := make([]int, bins)
	peak := 1
	for _, e := range timeline.Events {
		bucket := int(e.TimeMs * int64(bins) / maxTime(1, timeline.Duration+1))
		if bucket >= bins {
			bucket = bins - 1
		}
		density[bucket]++
		if density[bucket] > peak {
			peak = density[bucket]
		}
	}
	for i, n := range density {
		if n == 0 {
			continue
		}
		height := maxInt(2, int(math.Round(float64(n)*float64(trackH-10)/float64(peak))))
		xx := trackX + i*trackW/bins
		width := maxInt(1, trackW/bins-1)
		fill(hdc, xx, trackY+trackH-4-height, width, height, palette.accent2)
	}
	pointX := func(ms int64) int { return trackX + int(ms*int64(trackW)/timeline.Duration) }
	// Dim sections outside the trim and shade sections to be cut.
	if x := pointX(timeline.Start); x > trackX {
		fill(hdc, trackX, trackY, x-trackX, trackH, 0x31364B)
	}
	if x := pointX(timeline.End); x < trackX+trackW {
		fill(hdc, x, trackY, trackX+trackW-x, trackH, 0x31364B)
	}
	for _, c := range timeline.Cuts {
		l, r := pointX(c.StartMs), pointX(c.EndMs)
		fill(hdc, l, trackY, maxInt(2, r-l), trackH, 0x75506A)
	}
	fill(hdc, pointX(timeline.Start), trackY-3, 3, trackH+6, palette.green)
	fill(hdc, pointX(timeline.End)-2, trackY-3, 3, trackH+6, palette.red)
	if timeline.HaveCutIn {
		fill(hdc, pointX(timeline.CutIn), trackY, 2, trackH, palette.accent)
	}
	if timeline.HaveCutOut {
		fill(hdc, pointX(timeline.CutOut), trackY, 2, trackH, palette.accent)
	}
	fill(hdc, pointX(timeline.Cursor)-1, trackY-7, 3, trackH+14, palette.white)
	hit("tl-scrub", trackX-3, trackY-7, trackW+6, trackH+14)
	textDraw(hdc, "00:00", 311, 446, 92, 21, palette.subtle, app.microFont, DT_LEFT)
	textDraw(hdc, "CURSOR "+lyreTimeText(timeline.Cursor), trackX+trackW/2-120, 446, 240, 23, palette.white, app.itemFont, DT_CENTER)
	textDraw(hdc, lyreTimeText(timeline.Duration), trackX+trackW-95, 446, 95, 21, palette.subtle, app.microFont, DT_RIGHT)
	// Fixed +/- nudges make hundredth-of-a-second selection possible without
	// requiring users to enter timestamps by hand.
	for i, p := range []struct{ id, label string }{
		{"tl-10", "-10 s"}, {"tl-1", "-1 s"}, {"tl-01", "-0.1 s"},
		{"tl+01", "+0.1 s"}, {"tl+1", "+1 s"}, {"tl+10", "+10 s"},
	} {
		button(hdc, p.id, p.label, 310+i*116, 475, 108, 31, 0)
	}
	for i, p := range []struct{ id, label string }{
		{"tl-start", "SET START"}, {"tl-end", "SET END"}, {"tl-in", "MARK CUT IN"}, {"tl-out", "MARK CUT OUT"},
	} {
		button(hdc, p.id, p.label, 309+i*176, 514, 166, 35, 1)
	}
	for i, p := range []struct{ id, label string }{
		{"tl-add", "ADD CUT"}, {"tl-undo", "UNDO LAST CUT"}, {"tl-clear", "CLEAR CUTS"}, {"tl-reset", "RESET ALL"},
	} {
		button(hdc, p.id, p.label, 309+i*176, 557, 166, 35, 0)
	}
	button(hdc, "tl-save", "SAVE EDITED COPY", 309, 603, 249, 38, 2)
	button(hdc, "tl-back", "BACK TO LIBRARY", 568, 603, 213, 38, 0)
}
