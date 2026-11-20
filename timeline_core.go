package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Timeline Lab is a nondestructive editor for the same validated, timed AHK
// note arrays supported by Tempo Lab. No arbitrary AHK commands are edited.
type lyreTimedEvent struct {
	TimeMs int64
	Keys   string
	Line   int
}
type lyreTimeCut struct{ StartMs, EndMs int64 }
type lyreTimelineReport struct {
	OriginalEvents, SavedEvents, RemovedEvents int
	OriginalDurationMs, NewDurationMs          int64
	RemovedDurationMs                          int64
}

func parseLyreTimedSong(source string) ([]lyreTimedEvent, error) {
	if tempoEscHotkey.MatchString(source) || tempoEscKeyCall.MatchString(source) {
		return nil, errors.New("this script binds Escape; Timeline Lab does not edit Escape-binding macros")
	}
	lines := strings.SplitAfter(source, "\n")
	inArray, seen, closed := false, false, false
	var last int64 = -1
	events := make([]lyreTimedEvent, 0, 512)
	for i, line := range lines {
		body := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if !inArray {
			if tempoArrayBegin.MatchString(body) {
				if seen {
					return nil, errors.New("multiple timed-song arrays found")
				}
				seen, inArray = true, true
			}
			continue
		}
		if tempoArrayEnd.MatchString(body) {
			inArray = false
			closed = true
			continue
		}
		if strings.TrimSpace(body) == "" || strings.HasPrefix(strings.TrimSpace(body), ";") {
			continue
		}
		parts := tempoNoteLine.FindStringSubmatch(body)
		if parts == nil {
			return nil, fmt.Errorf("unsupported note array line %d", i+1)
		}
		ms, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || ms < 0 || ms > 24*60*60*1000 || ms < last {
			return nil, fmt.Errorf("invalid or out-of-order note time on line %d", i+1)
		}
		events = append(events, lyreTimedEvent{TimeMs: ms, Keys: parts[4], Line: i})
		last = ms
	}
	if inArray || !closed || len(events) < 2 {
		return nil, errors.New("no complete timed note array with at least two events found")
	}
	return events, nil
}

func validateLyreTimeCuts(start, end int64, cuts []lyreTimeCut) error {
	if start < 0 || end <= start {
		return errors.New("end marker must be later than start marker")
	}
	if len(cuts) > 200 {
		return errors.New("too many cuts")
	}
	var previousEnd int64 = start
	for _, c := range cuts {
		if c.StartMs < start || c.EndMs > end || c.EndMs <= c.StartMs || c.StartMs < previousEnd {
			return errors.New("cuts must be inside the selected range, ordered, and non-overlapping")
		}
		previousEnd = c.EndMs
	}
	return nil
}

// All cut intervals are [start,end); event exactly at cut end is retained.
// Any retained event is rebased to time zero and gaps are closed. The original
// source remains immutable, including its hotkeys and non-note code.
func cutLyreTimedSong(source string, start, end int64, cuts []lyreTimeCut) (string, lyreTimelineReport, error) {
	report := lyreTimelineReport{}
	events, err := parseLyreTimedSong(source)
	if err != nil {
		return "", report, err
	}
	report.OriginalEvents = len(events)
	report.OriginalDurationMs = events[len(events)-1].TimeMs
	if end > report.OriginalDurationMs {
		return "", report, errors.New("end marker exceeds song duration")
	}
	if err = validateLyreTimeCuts(start, end, cuts); err != nil {
		return "", report, err
	}
	for _, c := range cuts {
		report.RemovedDurationMs += c.EndMs - c.StartMs
	}
	lines := strings.SplitAfter(source, "\n")
	keep := make(map[int]int64, len(events))
	for _, event := range events {
		if event.TimeMs < start || event.TimeMs > end {
			continue
		}
		excluded := false
		var passed int64
		for _, c := range cuts {
			if event.TimeMs >= c.StartMs && event.TimeMs < c.EndMs {
				excluded = true
				break
			}
			if event.TimeMs >= c.EndMs {
				passed += c.EndMs - c.StartMs
			}
		}
		if excluded {
			continue
		}
		newTime := event.TimeMs - start - passed
		if newTime < 0 {
			return "", report, errors.New("edited note time became negative")
		}
		keep[event.Line] = newTime
	}
	if len(keep) < 2 {
		return "", report, errors.New("selected timeline contains fewer than two notes")
	}
	var lastNew int64 = -1
	for _, event := range events {
		ms, retained := keep[event.Line]
		if !retained {
			lines[event.Line] = ""
			continue
		}
		if ms < lastNew {
			return "", report, errors.New("resulting note order is invalid")
		}
		lastNew = ms
		line := lines[event.Line]
		body := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		suffix := line[len(body):]
		parts := tempoNoteLine.FindStringSubmatch(body)
		if parts == nil {
			return "", report, errors.New("note line changed while processing")
		}
		lines[event.Line] = parts[1] + strconv.FormatInt(ms, 10) + parts[3] + parts[4] + parts[5] + suffix
	}
	report.SavedEvents = len(keep)
	report.RemovedEvents = report.OriginalEvents - report.SavedEvents
	report.NewDurationMs = lastNew
	result := strings.Join(lines, "")
	newline := "\n"
	if strings.Contains(source, "\r\n") {
		newline = "\r\n"
	}
	if !strings.HasSuffix(result, "\n") {
		result += newline
	}
	result += fmt.Sprintf("; Cazy Lyre Timeline Lab: kept %d..%d ms, %d removed section(s). Original file preserved.%s", start, end, len(cuts), newline)
	return result, report, nil
}
