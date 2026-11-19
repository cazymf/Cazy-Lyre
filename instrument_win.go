//go:build windows

package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"
)

//go:embed capture_lyre.py
var captureScript []byte

//go:embed INSTALL_CAPTURE_ENGINE.cmd
var captureInstallScript []byte

type captureUpdate struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	Key      string `json:"key"`
	Index    int    `json:"index"`
	Total    int    `json:"total"`
	Folder   string `json:"folder"`
	Seconds  int    `json:"seconds"`
	Finished bool   `json:"-"`
	Failed   bool   `json:"-"`
}

var captureUpdates = make(chan captureUpdate, 96)

func instrumentDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Cazy Lyre", "instrument_samples", "Windsong Lyre")
}
func captureEnginePython() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Cazy Lyre", "capture_engine", "Scripts", "python.exe")
}
func instrumentReady() bool {
	dir := instrumentDir()
	if !fileExists(filepath.Join(dir, "instrument.json")) {
		return false
	}
	for _, key := range "QWERTYUASDFGHJZXCVBNM" {
		if !fileExists(filepath.Join(dir, fmt.Sprintf("%c.wav", key))) {
			return false
		}
	}
	return true
}

func openCaptureInstall() {
	installDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Cazy Lyre", "Lyre Studio")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		status("Cannot create installer: " + err.Error())
		return
	}
	script := filepath.Join(installDir, "INSTALL_CAPTURE_ENGINE.cmd")
	if err := os.WriteFile(script, captureInstallScript, 0644); err != nil {
		status("Cannot write installer: " + err.Error())
		return
	}
	verb, path := utf("open"), utf(script)
	r, _, _ := pShellExecuteW.Call(app.hwnd, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(path)), 0, 0, SW_SHOWNORMAL)
	if r <= 32 {
		status("Cannot open capture installer; run INSTALL_CAPTURE_ENGINE.cmd from the ZIP.")
		return
	}
	status("Capture setup opened. Install once, then come back and click START CAPTURE.")
}

func emitCaptureUpdate(h uintptr, update captureUpdate) {
	captureUpdates <- update
	pPostMessageW.Call(h, WM_APP+7, 0, 0)
}

func beginInstrumentCapture() {
	if app.captureBusy {
		status("Instrument capture already running. Use CANCEL to stop it.")
		return
	}
	if activeCount() > 0 {
		status("Stop your running macros before capturing individual lyre samples.")
		return
	}
	interpreter := findAHK()
	if interpreter == "" {
		status("AutoHotkey v2 not found. Install or select the interpreter in Settings.")
		return
	}
	python := captureEnginePython()
	if !fileExists(python) {
		status("Install the capture engine first: click INSTALL CAPTURE ENGINE.")
		return
	}
	work := filepath.Join(os.Getenv("LOCALAPPDATA"), "Cazy Lyre", "Lyre Studio")
	if err := os.MkdirAll(work, 0755); err != nil {
		status(err.Error())
		return
	}
	script := filepath.Join(work, "capture_lyre.py")
	if err := os.WriteFile(script, captureScript, 0644); err != nil {
		status(err.Error())
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	app.captureCancel = cancel
	app.captureBusy = true
	app.captureProgress = 0
	app.captureLastKey = ""
	app.captureState = "Starting live capture. Return to Genshin when the countdown begins."
	status("Capture launching. Keep Genshin open with the Windsong Lyre equipped, and SFX on.")
	hwnd := app.hwnd
	output := instrumentDir()
	go func() {
		defer cancel()
		cmd := exec.CommandContext(ctx, python, script, "--out", output, "--ahk", interpreter, "--countdown", "7")
		cmd.Env = append(cmd.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1", "PYTHONUNBUFFERED=1")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			emitCaptureUpdate(hwnd, captureUpdate{Message: "Capture engine failed: " + err.Error(), Failed: true, Finished: true})
			return
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err = cmd.Start(); err != nil {
			emitCaptureUpdate(hwnd, captureUpdate{Message: "Capture engine couldn't start: " + err.Error(), Failed: true, Finished: true})
			return
		}
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 128*1024)
		completed := false
		lastError := ""
		for scanner.Scan() {
			var update captureUpdate
			if json.Unmarshal(scanner.Bytes(), &update) == nil && update.Type != "" {
				if update.Type == "complete" {
					completed = true
				}
				if update.Type == "error" {
					lastError = update.Message
				}
				emitCaptureUpdate(hwnd, update)
			}
		}
		err = cmd.Wait()
		if ctx.Err() != nil {
			emitCaptureUpdate(hwnd, captureUpdate{Message: "Capture cancelled. Previous instrument samples were preserved.", Finished: true, Failed: true})
			return
		}
		if err != nil || !completed {
			msg := lastError
			if msg == "" {
				msg = strings.TrimSpace(stderr.String())
			}
			if msg == "" && scanner.Err() != nil {
				msg = scanner.Err().Error()
			}
			if msg == "" && err != nil {
				msg = err.Error()
			}
			if msg == "" {
				msg = "The capture process exited without a completed instrument profile."
			}
			emitCaptureUpdate(hwnd, captureUpdate{Message: "Capture unsuccessful: " + byteshort(msg, 240), Finished: true, Failed: true})
			return
		}
		if !instrumentReady() {
			emitCaptureUpdate(hwnd, captureUpdate{Message: "Capture completed but one or more WAV files are missing. Existing profile kept.", Finished: true, Failed: true})
			return
		}
		emitCaptureUpdate(hwnd, captureUpdate{Message: "Captured all 21 Windsong Lyre notes. Your instrument profile is saved locally.", Finished: true})
	}()
}

func cancelInstrumentCapture() {
	if !app.captureBusy || app.captureCancel == nil {
		status("There is no active capture to stop.")
		return
	}
	app.captureCancel()
	app.captureState = "Stopping capture..."
	status("Cancelling capture; previously saved instrument profile is retained.")
}

func openInstrumentFolder() {
	folder := instrumentDir()
	if !instrumentReady() {
		status("No complete instrument profile yet. Capture all 21 notes first.")
		return
	}
	verb, path := utf("open"), utf(folder)
	r, _, _ := pShellExecuteW.Call(app.hwnd, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(path)), 0, 0, SW_SHOWNORMAL)
	if r <= 32 {
		status("Couldn't open instrument folder: " + folder)
	}
}

func handleCaptureUpdate() {
	select {
	case update := <-captureUpdates:
		if update.Index > app.captureProgress {
			app.captureProgress = update.Index
		}
		if update.Key != "" {
			app.captureLastKey = update.Key
		}
		if update.Message != "" {
			app.captureState = update.Message
		}
		if update.Finished {
			app.captureBusy = false
			app.captureCancel = nil
			if update.Failed {
				status(app.captureState)
				return
			}
		}
		if update.Type == "error" {
			status(app.captureState)
			return
		}
		if update.Type == "countdown" && update.Seconds > 0 {
			status(fmt.Sprintf("Genshin capture starts in %d sec. Switch to Genshin now!", update.Seconds))
			return
		}
		if update.Type == "note" {
			status(fmt.Sprintf("Windsong Lyre: captured key %s — %d / 21", update.Key, app.captureProgress))
			return
		}
		if update.Finished {
			status(app.captureState)
			return
		}
		redraw()
	default:
	}
}

func paintInstrument(hdc uintptr, w, h int) {
	rounded(hdc, 283, 281, w-315, 169, palette.panel, palette.border, 16)
	textDraw(hdc, "ONE-TIME WINDS WINDOWS AUDIO CALIBRATION", 304, 291, w-370, 28, palette.accent2, app.microFont, DT_LEFT)
	textDraw(hdc, "1. Open Genshin, equip Windsong Lyre, set MUSIC 0 and keep SFX audible.", 304, 319, w-362, 25, palette.white, app.subFont, DT_LEFT)
	textDraw(hdc, "2. Click START CAPTURE, then switch to Genshin before the 7-second countdown ends.", 304, 346, w-362, 25, palette.white, app.subFont, DT_LEFT)
	textDraw(hdc, "3. Cazy Lyre plays Q-M one at a time while recording Windows speaker audio.", 304, 373, w-362, 25, palette.white, app.subFont, DT_LEFT)
	textDraw(hdc, "No screen sharing. No OpenAI credits. No Escape key. Keep the game focused.", 304, 412, w-355, 25, palette.green, app.subFont, DT_LEFT)

	rounded(hdc, 283, 459, w-315, 168, palette.panel, palette.border, 16)
	textDraw(hdc, "CAPTURE ENGINE", 304, 466, 230, 24, palette.accent2, app.microFont, DT_LEFT)
	engineText := "Not installed - click INSTALL CAPTURE ENGINE"
	if fileExists(captureEnginePython()) {
		engineText = "Python capture engine installed"
	}
	textDraw(hdc, engineText, 304, 489, w-352, 24, palette.white, app.subFont, DT_LEFT)
	button(hdc, "instrument-install", "INSTALL CAPTURE ENGINE", 304, 516, 221, 34, 0)
	if app.captureBusy {
		button(hdc, "instrument-cancel", "CANCEL CAPTURE", 533, 516, 207, 34, 3)
	} else {
		button(hdc, "instrument-start", "START 21-NOTE CAPTURE", 533, 516, 207, 34, 2)
	}
	button(hdc, "instrument-open", "OPEN SAMPLES", 747, 516, 165, 34, 1)
	recorded := app.captureProgress
	if !app.captureBusy && instrumentReady() {
		recorded = 21
	}
	state := "Instrument not calibrated yet"
	if instrumentReady() {
		state = "Saved profile ready: 21 WAV recordings"
	}
	if app.captureState != "" {
		state = app.captureState
	}
	textDraw(hdc, fmt.Sprintf("SAMPLES: %d / 21", recorded), 304, 561, 196, 27, palette.accent2, app.itemFont, DT_LEFT)
	textDraw(hdc, byteshort(state, 130), 304, 588, w-362, 31, palette.muted, app.subFont, DT_LEFT)

	rounded(hdc, 283, 640, w-315, 50, palette.panel, palette.border, 12)
	textDraw(hdc, "Audio is captured locally from your DEFAULT WINDOWS SPEAKER; close other sound apps.", 302, 647, w-366, 20, palette.white, app.subFont, DT_LEFT)
	textDraw(hdc, "Only a completed 21-note capture replaces an earlier sample library.", 302, 668, w-365, 18, palette.muted, app.microFont, DT_LEFT)
	_ = h
}

// The setup script runs only when clicked; merely opening this page never
// installs libraries or starts the recording process.
