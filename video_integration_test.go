package main

import (
	"path/filepath"
"os"
	"testing"
)

func TestRealExtractedVideoMIDI(t *testing.T) {
	cases := []struct {
		path string
		min  int
	}{
		{"test_river/visual.mid", 65},
		{"test_chasing/visual.mid", 200},
		{"test_capitano/visual.mid", 150},
	}
	for _, c := range cases {
		n, e := parseMIDIFile(filepath.Clean(c.path))
		if e != nil {
            if os.IsNotExist(e) { t.Skipf("previous release did not bundle the original visual MIDI test fixtures (%s)", c.path) }
			t.Fatalf("cannot read %s: %v", c.path, e)
		}
		if len(n) < c.min {
			t.Errorf("%s: %d note onsets, expected >=%d", c.path, len(n), c.min)
		}
		for i := 1; i < len(n); i++ {
			if n[i].Ms < n[i-1].Ms {
				t.Errorf("%s unsorted events", c.path)
				break
			}
		}
	}
}
