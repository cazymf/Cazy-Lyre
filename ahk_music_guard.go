package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// AI fixes must never regenerate or shorten a song's timestamped note arrays.
// These pieces are immutable musical data. Only playback/control code is sent
// for editing, with uniquely labeled stand-ins for the original arrays.
type preservedMusicBlock struct {
	Original string
	StandIn  string
	Count    int
}

var musicArrayHeaderRE = regexp.MustCompile(`(?im)\b(?:song|notes|events|melody|noteevents|note_events|songdata|notedata)\s*:=\s*\[`)
var songEventRE = regexp.MustCompile(`\[\s*\d+(?:\.\d+)?\s*,\s*"[^"\r\n]{1,40}"`)
var playableActionRE = regexp.MustCompile(`(?im)(?:^|[\s\{])(?:Send|SendEvent|SendInput|SendText|ControlSend|Click|MouseClick)\s*(?:\(|\s+[^\r\n])`)

// Array bracket matching ignores brackets inside AHK quoted strings and line
// comments, so a chord or comment can't prematurely end the protected block.
func ahkArrayEnd(source string, start int) (int, error) {
	depth, quoted, commented := 0, false, false
	for i := start; i < len(source); i++ {
		c := source[i]
		if commented {
			if c == '\n' {
				commented = false
			}
			continue
		}
		if quoted {
			if c == '`' && i+1 < len(source) {
				i++
				continue
			}
			if c == '"' {
				if i+1 < len(source) && source[i+1] == '"' {
					i++
					continue
				}
				quoted = false
			}
			continue
		}
		switch c {
		case ';':
			commented = true
		case '"':
			quoted = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i + 1, nil
			}
			if depth < 0 {
				return 0, errors.New("unexpected closing bracket in note array")
			}
		}
	}
	return 0, errors.New("unterminated note array in selected AHK")
}

func protectLyreNotes(src string) (string, []preservedMusicBlock, int, error) {
	found := musicArrayHeaderRE.FindAllStringIndex(src, 100)
	if len(found) == 0 {
		return src, nil, len(songEventRE.FindAllStringIndex(src, -1)), nil
	}
	var out strings.Builder
	var blocks []preservedMusicBlock
	last := 0
	for _, m := range found {
		if m[0] < last {
			continue
		}
		originalHeader := src[m[0]:m[1]]
		firstBracket := m[1] - 1
		end, err := ahkArrayEnd(src, firstBracket)
		if err != nil {
			return "", nil, 0, err
		}
		original := src[m[0]:end]
		count := len(songEventRE.FindAllStringIndex(original, -1))
		if count == 0 {
			continue
		}
		varName := strings.TrimSpace(strings.SplitN(originalHeader, ":=", 2)[0])
		mark := fmt.Sprintf("__CAZY_LYRE_IMMUTABLE_MUSIC_DATA_%03d__", len(blocks)+1)
		standIn := varName + ` := "` + mark + `"`
		out.WriteString(src[last:m[0]])
		out.WriteString(standIn)
		last = end
		blocks = append(blocks, preservedMusicBlock{Original: original, StandIn: standIn, Count: count})
		if len(blocks) > 20 {
			return "", nil, 0, errors.New("too many independent note arrays to protect")
		}
	}
	out.WriteString(src[last:])
	sum := 0
	for _, b := range blocks {
		sum += b.Count
	}
	if len(blocks) == 0 {
		sum = len(songEventRE.FindAllStringIndex(src, -1))
	}
	return out.String(), blocks, sum, nil
}

func restoreLyreNotes(generated string, blocks []preservedMusicBlock, sourceEvents int) (string, error) {
	output := generated
	for _, block := range blocks {
		if strings.Count(output, block.StandIn) != 1 {
			return "", fmt.Errorf("AI response omitted or changed a protected song array (%d musical events). Original file remains unchanged; try FIX SELECTED again", block.Count)
		}
		output = strings.Replace(output, block.StandIn, block.Original, 1)
	}
	if sourceEvents > 0 {
		got := len(songEventRE.FindAllStringIndex(output, -1))
		if got < sourceEvents {
			return "", fmt.Errorf("AI response lost musical notes (%d of %d events remain). It was NOT saved", got, sourceEvents)
		}
	}
	return output, nil
}

// Lightweight structural check catches clipped AI responses. Braces in AHK
// string literals (e.g., "{f down}") do not count as code delimiters.
func ahkBalancedBraces(source string) bool {
	depth := 0
	quote := byte(0)
	commented := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if commented {
			if c == '\n' {
				commented = false
			}
			continue
		}
		if quote != 0 {
			if c == '`' && i+1 < len(source) {
				i++
				continue
			}
			if c == quote {
				if i+1 < len(source) && source[i+1] == quote {
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case ';':
			commented = true
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0 && quote == 0
}

func validateNonemptyAHK(content string) error {
	if len(strings.TrimSpace(content)) < 100 {
		return errors.New("AI returned a tiny script. Nothing was saved; retry FIX SELECTED")
	}
	if !strings.Contains(content, "::") && !strings.Contains(content, "Hotkey(") {
		return errors.New("the output has no keyboard/mouse hotkey; it may not run")
	}
	if !playableActionRE.MatchString(content) {
		return errors.New("AI returned a script with no key-sending action, so it would play silently. Nothing was saved")
	}
	if !ahkBalancedBraces(content) {
		return errors.New("AI returned incomplete or unbalanced AHK code. Nothing was saved")
	}
	return nil
}
