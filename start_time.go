package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const maxLyreSkipSeconds = 7200

// parseLyreStartTime understands MM:SS, HH:MM:SS, whole seconds, and
// the user's MM.SS shorthand (e.g. 1.40 means 1 minute 40 seconds).
// Decimal fractions such as 1.5 still mean 1.5 seconds.
func parseLyreStartTime(value string) (int64, error) {
	s := strings.TrimSpace(value)
	if s == "" {
		return 0, fmt.Errorf("enter a start time, for example 1:40")
	}
	var seconds float64
	parts := strings.Split(s, ":")
	if len(parts) == 2 || len(parts) == 3 {
		vals := make([]float64, len(parts))
		for i, part := range parts {
			if part == "" {
				return 0, fmt.Errorf("use MM:SS, such as 1:40")
			}
			var err error
			vals[i], err = strconv.ParseFloat(part, 64)
			if err != nil || math.IsNaN(vals[i]) || math.IsInf(vals[i], 0) || vals[i] < 0 {
				return 0, fmt.Errorf("invalid start time %q; use MM:SS", s)
			}
			if i < len(parts)-1 && (strings.Contains(part, ".") || vals[i] != math.Trunc(vals[i])) {
				return 0, fmt.Errorf("only the seconds field can have a decimal")
			}
		}
		last := vals[len(vals)-1]
		if last >= 60 {
			return 0, fmt.Errorf("seconds must be less than 60 in MM:SS")
		}
		if len(parts) == 2 {
			seconds = vals[0]*60 + last
		} else {
			if vals[1] >= 60 {
				return 0, fmt.Errorf("minutes must be less than 60 in HH:MM:SS")
			}
			seconds = vals[0]*3600 + vals[1]*60 + last
		}
	} else if len(parts) > 3 {
		return 0, fmt.Errorf("use MM:SS, such as 1:40")
	} else if dot := strings.IndexByte(s, '.'); dot >= 0 && len(s[dot+1:]) == 2 {
		// Interpret 1.40 as 1:40. Reject ambiguous seconds >59 so the
		// converter never silently starts at the wrong part of the song.
		minStr, secStr := s[:dot], s[dot+1:]
		mins, err1 := strconv.Atoi(minStr)
		secs, err2 := strconv.Atoi(secStr)
		if err1 != nil || err2 != nil || mins < 0 || secs < 0 || secs >= 60 {
			return 0, fmt.Errorf("invalid MM.SS time %q; use 1:40 or whole seconds", s)
		}
		seconds = float64(mins*60 + secs)
	} else if len(parts) == 1 {
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
			return 0, fmt.Errorf("invalid start time %q; use 1:40 or 100", s)
		}
		seconds = n
	}
	if seconds < 0 || seconds > maxLyreSkipSeconds {
		return 0, fmt.Errorf("start time must be between 0:00 and 120:00")
	}
	return int64(math.Round(seconds * 1000)), nil
}

func formatLyreStartTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	seconds := (ms + 500) / 1000
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func nudgeLyreStartTime(input string, seconds int64) (string, error) {
	start, err := parseLyreStartTime(input)
	if err != nil {
		return "", err
	}
	start += seconds * 1000
	if start < 0 {
		start = 0
	}
	if start > maxLyreSkipSeconds*1000 {
		start = maxLyreSkipSeconds * 1000
	}
	return formatLyreStartTime(start), nil
}
