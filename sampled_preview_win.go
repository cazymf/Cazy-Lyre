//go:build windows

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

//go:embed sampled_preview.py
var sampledPreviewPython []byte

var pPlaySoundW = syscall.NewLazyDLL("winmm.dll").NewProc("PlaySoundW")

const (
	sndAsync     = 0x0001
	sndNoDefault = 0x0002
	sndFilename  = 0x00020000
)

func stopSampledSound() {
	pPlaySoundW.Call(0, 0, 0)
}

func beginSampledLyrePreview() {
	if len(app.lyrePrepared.Events) == 0 {
		status("Prepare a song before sampling the real lyre sounds.")
		return
	}
	if !instrumentReady() {
		status("Record the 21 lyre samples first: open LYRE CAPTURE in the sidebar.")
		return
	}
	python := captureEnginePython()
	if !fileExists(python) {
		status("Install the capture engine first in LYRE CAPTURE.")
		return
	}
	stopLyrePreview()
	ctx, cancel := context.WithCancel(context.Background())
	app.lyrePreviewCancel = cancel
	app.lyrePreviewID++
	id := app.lyrePreviewID
	score := app.lyrePrepared
	hwnd := app.hwnd
	status("Rendering your arrangement using the 21 real Genshin Windsong Lyre samples...")
	go func() {
		dir, err := os.MkdirTemp("", "cazy-sampled-preview-*")
		if err != nil {
			emitSampledPreviewResult(hwnd, id, err)
			return
		}
		defer os.RemoveAll(dir)
		script := filepath.Join(dir, "sampled_preview.py")
		if err = os.WriteFile(script, sampledPreviewPython, 0644); err != nil {
			emitSampledPreviewResult(hwnd, id, err)
			return
		}
		notes := filepath.Join(dir, "events.json")
		data, err := json.Marshal(score.Events)
		if err == nil {
			err = os.WriteFile(notes, data, 0644)
		}
		if err != nil {
			emitSampledPreviewResult(hwnd, id, err)
			return
		}
		output := filepath.Join(dir, "lyre_preview.wav")
		cmd := exec.CommandContext(ctx, python, script, "--events", notes, "--samples", instrumentDir(), "--out", output)
		cmd.Env = append(cmd.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
		msg, err := cmd.CombinedOutput()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			emitSampledPreviewResult(hwnd, id, fmt.Errorf("Sample renderer: %v (%s)", err, byteshort(string(msg), 240)))
			return
		}
		if ctx.Err() != nil {
			return
		}
		u16 := utf(output)
		ok, _, _ := pPlaySoundW.Call(uintptr(unsafe.Pointer(u16)), 0, sndFilename|sndAsync|sndNoDefault)
		if ok == 0 {
			emitSampledPreviewResult(hwnd, id, fmt.Errorf("Windows could not play the rendered WAV file"))
			return
		}
		// PlaySoundW async must keep the WAV file alive until playback finishes.
		duration := time.Duration(score.DurationMs+3500) * time.Millisecond
		if duration < 2*time.Second {
			duration = 2 * time.Second
		}
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			stopSampledSound()
			emitSampledPreviewResult(hwnd, id, nil)
		}
	}()
}

func emitSampledPreviewResult(hwnd uintptr, id uint64, err error) {
	previewResults <- previewResult{ID: id, Err: err}
	pPostMessageW.Call(hwnd, WM_APP+4, 0, 0)
}
