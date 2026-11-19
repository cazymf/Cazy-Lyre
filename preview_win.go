//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"syscall"
	"time"
	"unsafe"
)

// Built-in Windows software MIDI synthesizer preview. This is an approximation
// of the Genshin Windsong Lyre, not a recording of the game instrument.
var winmm = syscall.NewLazyDLL("winmm.dll")
var midiOutOpen = winmm.NewProc("midiOutOpen")
var midiOutClose = winmm.NewProc("midiOutClose")
var midiOutShortMsg = winmm.NewProc("midiOutShortMsg")
var midiOutReset = winmm.NewProc("midiOutReset")

type previewResult struct {
	ID  uint64
	Err error
}

var previewResults = make(chan previewResult, 8)

type midiPreviewAction struct {
	at    int64
	pitch int
	on    bool
}

func previewActions(score lyreScore) []midiPreviewAction {
	keyPitch := map[byte]int{}
	for _, x := range pitchKeys {
		keyPitch[x.key] = x.pitch
	}
	acts := make([]midiPreviewAction, 0, len(score.Events)*6)
	for _, ev := range score.Events {
		for i := 0; i < len(ev.Keys); i++ {
			if p, ok := keyPitch[ev.Keys[i]]; ok {
				acts = append(acts, midiPreviewAction{at: ev.Ms, pitch: p, on: true})
				acts = append(acts, midiPreviewAction{at: ev.Ms + 150, pitch: p, on: false})
			}
		}
	}
	sort.SliceStable(acts, func(i, j int) bool {
		if acts[i].at != acts[j].at {
			return acts[i].at < acts[j].at
		}
		if acts[i].on != acts[j].on {
			return !acts[i].on
		} // release before repeated press
		return acts[i].pitch < acts[j].pitch
	})
	return acts
}

func previewMusic(ctx context.Context, score lyreScore) error {
	var handle uintptr
	code, _, _ := midiOutOpen.Call(uintptr(unsafe.Pointer(&handle)), uintptr(^uint32(0)), 0, 0, 0)
	if code != 0 || handle == 0 {
		return fmt.Errorf("Windows MIDI synthesizer unavailable (code %d). Preview requires a working Windows MIDI output device", code)
	}
	defer midiOutClose.Call(handle)
	defer midiOutReset.Call(handle)
	// General MIDI program 46 (0-based) = orchestral harp.
	midiOutShortMsg.Call(handle, uintptr(0xC0|(46<<8)))
	acts := previewActions(score)
	if len(acts) == 0 {
		return errors.New("no notes to preview")
	}
	start := time.Now()
	for _, act := range acts {
		delay := time.Until(start.Add(time.Duration(act.at) * time.Millisecond))
		if delay > 0 {
			t := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				if !t.Stop() {
					select {
					case <-t.C:
					default:
					}
				}
				return nil
			case <-t.C:
			}
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		kind := 0x80
		velocity := 0
		if act.on {
			kind = 0x90
			velocity = 95
		}
		msg := uintptr(kind | (act.pitch << 8) | (velocity << 16))
		midiOutShortMsg.Call(handle, msg)
	}
	return nil
}
