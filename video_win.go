//go:build windows

package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

//go:embed video_notes.py
var videoExtractorSource []byte

const (
	videoVisual = "visual"
	videoOverlay = "overlay"
	videoHybrid = "hybrid"
	videoAudio  = "audio"
)

func normalizeVideoMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case videoOverlay:
		return videoOverlay
	case videoHybrid:
		return videoHybrid
	case videoAudio:
		return videoAudio
	default:
		return videoVisual
	}
}
func isVideoPath(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mp4", ".mkv", ".mov", ".webm", ".avi":
		return true
	}
	return false
}
func videoEngineDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Cazy Lyre", "video_engine")
}
func videoPython() string {
	p := filepath.Join(videoEngineDir(), "Scripts", "python.exe")
	if fileExists(p) {
		return p
	}
	return ""
}
func videoStudioDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Cazy Lyre", "Video Studio")
}

// Video engine is deliberately in a separate venv from TensorFlow/BasicPitch.
// Their numpy constraints are not compatible with every OpenCV version.
const videoSetup = `@echo off
setlocal
TITLE Cazy Lyre - Video Engine Setup
set "ENV=%LOCALAPPDATA%\Cazy Lyre\video_engine"
echo.
echo   CAZY LYRE - VIDEO ENGINE INSTALL
echo   =================================
echo   Local visual piano note recognition (OpenCV)
echo.
py -3.11 -V
if errorlevel 1 (
 echo Python 3.11 is required.
 pause
 exit /b 1
)
if not exist "%ENV%\Scripts\python.exe" (
 py -3.11 -m venv "%ENV%"
 if errorlevel 1 goto FAIL
)
"%ENV%\Scripts\python.exe" -m pip install --upgrade pip
if errorlevel 1 goto FAIL
"%ENV%\Scripts\python.exe" -m pip install "numpy==1.26.4" "opencv-python-headless==4.10.0.84" "imageio-ffmpeg>=0.5,<1"
if errorlevel 1 goto FAIL
"%ENV%\Scripts\python.exe" -c "import cv2,numpy,imageio_ffmpeg; print('Video engine ready: OpenCV',cv2.__version__)"
if errorlevel 1 goto FAIL
echo.
echo DONE. Import a Synthesia MP4 or Genshin Lyre key-overlay video in Cazy Lyre.
pause
exit /b 0
:FAIL
echo.
echo Video engine setup failed. See error details above.
pause
exit /b 1
`

func setupVideoEngine() {
	dir := videoStudioDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		status(err.Error())
		return
	}
	p := filepath.Join(dir, "INSTALL_VIDEO_ENGINE.cmd")
	if err := os.WriteFile(p, []byte(videoSetup), 0644); err != nil {
		status(err.Error())
		return
	}
	verb := utf("open")
	target := utf(p)
	r, _, _ := pShellExecuteW.Call(app.hwnd, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, SW_SHOWNORMAL)
	if r <= 32 {
		status("Unable to open Video Engine setup. Run the .cmd file from the ZIP.")
		return
	}
	status("Video setup opened. Install OpenCV, then import a piano tutorial or lyre-overlay video.")
}

func setVideoMode(mode string) {
	mode = normalizeVideoMode(mode)
	if app.videoMode == mode {
		return
	}
	app.videoMode = mode
	save()
	clearLyrePrepared()
	status("Video mode: " + strings.ToUpper(mode) + ". Select an MP4 and PREPARE PREVIEW. LYRE KEYS mode reads 21 glowing keys.")
}

func prepareVideoLyre(path string, skipMs int64, speed float64, quality string, mode string) (lyreScore, error) {
	mode = normalizeVideoMode(mode)
	py := videoPython()
	if py == "" {
		return lyreScore{}, errors.New("Video Engine missing. Click SETUP VIDEO ENGINE to install OpenCV before importing an MP4")
	}
	work, err := os.MkdirTemp("", "cazy-lyre-video-*")
	if err != nil {
		return lyreScore{}, err
	}
	defer os.RemoveAll(work)
	scriptPath := filepath.Join(work, "video_notes.py")
	if err = os.WriteFile(scriptPath, videoExtractorSource, 0600); err != nil {
		return lyreScore{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	args := []string{scriptPath, "--input", path, "--outdir", work, "--mode", mode, "--start-ms", fmt.Sprint(skipMs)}
	cmd := exec.CommandContext(ctx, py, args...)
	cmd.Dir = work
	cmd.Env = append(cmd.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1", "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		report := strings.TrimSpace(string(out))
		if len(report) > 1500 {
			report = report[len(report)-1500:]
		}
		if ctx.Err() != nil {
			return lyreScore{}, fmt.Errorf("video recognition timed out: %w", ctx.Err())
		}
		return lyreScore{}, fmt.Errorf("Video recognition failed: %s", report)
	}
	var notes []pianoEvent
	if mode != videoAudio {
		notes, err = parseMIDIFile(filepath.Join(work, "visual.mid"))
		if err != nil {
			return lyreScore{}, fmt.Errorf("could not read visual MIDI: %w", err)
		}
	}
	if mode == videoAudio || mode == videoHybrid {
		engine := findAudioEngine()
		if engine == "" {
			return lyreScore{}, errors.New("Audio and Hybrid video modes also require Basic Pitch. Click SETUP AUDIO ENGINE.")
		}
		wavPath := filepath.Join(work, "audio.wav")
		if !fileExists(wavPath) {
			return lyreScore{}, errors.New("Video audio extraction did not produce a WAV file")
		}
		audioOut := filepath.Join(work, "audio_midi")
		if err := os.MkdirAll(audioOut, 0700); err != nil {
			return lyreScore{}, err
		}
		audioCmd := exec.CommandContext(ctx, engine, audioOut, wavPath)
		audioCmd.Dir = work
		audioCmd.Env = append(audioCmd.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1", "TF_CPP_MIN_LOG_LEVEL=2", "NO_COLOR=1")
		audioLog, e := audioCmd.CombinedOutput()
		if e != nil {
			report := strings.TrimSpace(string(audioLog))
			if len(report) > 950 {
				report = report[len(report)-950:]
			}
			return lyreScore{}, fmt.Errorf("Audio transcription failed: %v %s", e, report)
		}
		entries, e := os.ReadDir(audioOut)
		if e != nil {
			return lyreScore{}, e
		}
		var audioPath string
		for _, f := range entries {
			if strings.EqualFold(filepath.Ext(f.Name()), ".mid") || strings.EqualFold(filepath.Ext(f.Name()), ".midi") {
				audioPath = filepath.Join(audioOut, f.Name())
				break
			}
		}
		if audioPath == "" {
			return lyreScore{}, errors.New("Basic Pitch produced no MIDI for the video soundtrack")
		}
		audioNotes, e := parseMIDIFile(audioPath)
		if e != nil {
			return lyreScore{}, e
		}
		if mode == videoAudio {
			notes = audioNotes
		} else {
			notes = mergeVideoAudioNotes(notes, audioNotes)
		}
	}
	return createLyreScoreQuality(notes, skipMs, speed, quality)
}

// Hybrid mode uses the screen-keyboard pitches as ground truth and applies
// small timing refinements when the audio model agrees in both pitch and time.
// Unsupported audio notes are NOT added: orchestral backing otherwise becomes
// spurious Genshin chords.
func mergeVideoAudioNotes(visual, audio []pianoEvent) []pianoEvent {
	if len(visual) == 0 {
		return audio
	}
	if len(audio) == 0 {
		return visual
	}
	out := make([]pianoEvent, len(visual))
	copy(out, visual)
	j := 0
	for i := range out {
		ms := out[i].Ms
		for j < len(audio) && audio[j].Ms < ms-200 {
			j++
		}
		bestDiff := int64(131)
		bestMS := ms
		for k := j; k < len(audio) && audio[k].Ms <= ms+130; k++ {
			if audio[k].Pitch != out[i].Pitch {
				continue
			}
			diff := audio[k].Ms - ms
			if diff < 0 {
				diff = -diff
			}
			if diff < bestDiff {
				bestDiff = diff
				bestMS = audio[k].Ms
			}
		}
		if bestDiff <= 130 {
			out[i].Ms = (ms*4 + bestMS) / 5
		}
	}
	return out
}
