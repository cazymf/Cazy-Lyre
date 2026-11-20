package main

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Tempo Lab edits ONLY note timestamps in recognized AHK v2 music event arrays.
// The original is never overwritten, hotkeys and key combinations are untouched.
var tempoArrayBegin = regexp.MustCompile(`(?i)^[ \t]*(?:global[ \t]+)?(?:notes|song|noteevents|events|songevents)[ \t]*:=[ \t]*\[[ \t]*$`)
var tempoArrayEnd = regexp.MustCompile(`^[ \t]*\][ \t]*(?:;[^\r\n]*)?$`)
var tempoNoteLine = regexp.MustCompile(`(?i)^([ \t]*\[[ \t]*)([0-9]+)([ \t]*,[ \t]*")([QWERTYUASDFGHJZXCVBNM]+)("[ \t]*\][ \t]*,?[ \t]*(?:;[^\r\n]*)?)$`)
var tempoEscHotkey = regexp.MustCompile(`(?im)^[ \t]*(?:[$~*^!+#<>])*[ \t]*Esc[ \t]*::`)
var tempoEscKeyCall = regexp.MustCompile(`(?i)(?:Hotkey|Send|SendEvent|SendInput)[ \t]*\([ \t]*"(?:\{)?Esc(?:\})?`)

type tempoReport struct {
	Events             int
	OriginalDurationMs int64
	NewDurationMs      int64
	Speed              float64
}

func validTempoSpeed(speed float64) bool {
	switch speed {
	case 1.0, 1.10, 1.25, 1.35, 1.5, 1.75, 2.0:
		return true
	}
	return false
}

func tempoSpeedLabel(speed float64) string {
	return strconv.FormatFloat(speed, 'f', 2, 64) + "x"
}

func tempoVariantSuffix(speed float64) string {
	s := strconv.FormatFloat(speed, 'f', 2, 64)
	return strings.ReplaceAll(s, ".", "_") + "x"
}

func applySongTempo(source string, speed float64) (string, tempoReport, error) {
	report := tempoReport{Speed: speed}
	if !validTempoSpeed(speed) {
		return "", report, errors.New("unsupported tempo preset")
	}
	if tempoEscHotkey.MatchString(source) || tempoEscKeyCall.MatchString(source) {
		return "", report, errors.New("this script binds Esc; Tempo Lab will not reproduce an Esc hotkey")
	}
	// Keep the exact line ending / formatting / other AHK code from the input.
	lines := strings.SplitAfter(source, "\n")
	inArray := false
	seenArray := false
	finished := false
	var lastMs int64 = -1
	var lastScaled int64 = -1
	for i, line := range lines {
		body := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		lineEnding := line[len(body):]
		if !inArray {
			if tempoArrayBegin.MatchString(body) {
				if seenArray {
					return "", report, errors.New("multiple timed-song arrays detected; cannot safely choose one")
				}
				inArray, seenArray = true, true
			}
			continue
		}
		if tempoArrayEnd.MatchString(body) {
			inArray = false
			finished = true
			continue
		}
		if strings.TrimSpace(body) == "" || strings.HasPrefix(strings.TrimSpace(body), ";") {
			continue
		}
		fields := tempoNoteLine.FindStringSubmatch(body)
		if fields == nil {
			return "", report, fmt.Errorf("unsupported note-array line %d: only [milliseconds, \"lyrekeys\"] is supported", i+1)
		}
		ms, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || ms > 24*60*60*1000 {
			return "", report, fmt.Errorf("invalid note timestamp on line %d", i+1)
		}
		if ms < lastMs {
			return "", report, fmt.Errorf("song note times go backward on line %d", i+1)
		}
		lastMs = ms
		adjusted := int64(math.Round(float64(ms) / speed))
		if adjusted < lastScaled {
			return "", report, fmt.Errorf("tempo changed note ordering on line %d", i+1)
		}
		lastScaled = adjusted
		lines[i] = fields[1] + strconv.FormatInt(adjusted, 10) + fields[3] + fields[4] + fields[5] + lineEnding
		report.Events++
		report.OriginalDurationMs = ms
		report.NewDurationMs = adjusted
	}
	if inArray || !finished || report.Events == 0 {
		return "", report, errors.New("no complete timed-song array found; import a lyre AHK using notes :=, Song :=, NoteEvents := or events :=")
	}
	if report.Events < 2 {
		return "", report, errors.New("song has fewer than two timed note events")
	}
	// Attach metadata only as a comment. The script's controls remain identical.
	var ending string
	if strings.Contains(source, "\r\n") {
		ending = "\r\n"
	} else {
		ending = "\n"
	}
	result := strings.Join(lines, "")
	if !strings.HasSuffix(result, "\n") {
		result += ending
	}
	result += fmt.Sprintf("; Cazy Lyre v3.7 Tempo Lab: %.2fx. Source macro was not modified.%s", speed, ending)
	return result, report, nil
}
