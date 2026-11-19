//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type lyreResult struct {
	Path  string
	Title string
	Mode  string
	Err   error
	Score lyreScore
}

var lyreResults = make(chan lyreResult, 3)

func lyreAppDir() string { return filepath.Join(filepath.Dir(appDataPath()), "Lyre Studio") }
func localAudioEngine() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Cazy Lyre", "audio_engine", "Scripts", "basic-pitch.exe")
}
func findAudioEngine() string {
	if p := localAudioEngine(); fileExists(p) {
		return p
	}
	if p, err := exec.LookPath("basic-pitch.exe"); err == nil {
		return p
	}
	if p, err := exec.LookPath("basic-pitch"); err == nil {
		return p
	}
	return ""
}
func fileExists(p string) bool { st, e := os.Stat(p); return e == nil && !st.IsDir() }

// A one-click installer for users with Python 3.11 (64-bit) installed.
// Runs only when they explicitly choose the SETUP AUDIO action.
const basicPitchSetup = `@echo off
setlocal
TITLE Cazy Lyre - Install Audio Transcription
set "ENV=%LOCALAPPDATA%\Cazy Lyre\audio_engine"
echo.
echo  CAZY LYRE - AUDIO ENGINE SETUP
echo  ===============================
echo.
echo  Requires Python 3.11 (64-bit) from https://www.python.org/downloads/
echo  Installs Spotify Basic Pitch locally; no OpenAI API credits needed.
echo.
py -3.11 -V
if errorlevel 1 (
 echo.
 echo  Python 3.11 is missing. Install it, then run this setup again.
 pause
 exit /b 1
)
if not exist "%ENV%\Scripts\python.exe" (
 py -3.11 -m venv "%ENV%"
 if errorlevel 1 goto FAIL
)
"%ENV%\Scripts\python.exe" -m pip install --upgrade pip
if errorlevel 1 goto FAIL
"%ENV%\Scripts\python.exe" -m pip install "basic-pitch[onnx]"
if errorlevel 1 goto FAIL
if not exist "%ENV%\Scripts\basic-pitch.exe" goto FAIL
echo.
echo SUCCESS - Audio engine is installed! Open Cazy Lyre and import your MP3.
pause
exit /b 0
:FAIL
echo.
echo Setup failed. See errors above. You can also install a Basic Pitch compatible Python environment manually.
pause
exit /b 1
`

func setupAudioEngine() {
	dir := lyreAppDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		status(err.Error())
		return
	}
	path := filepath.Join(dir, "INSTALL_AUDIO_ENGINE.cmd")
	if err := os.WriteFile(path, []byte(basicPitchSetup), 0644); err != nil {
		status(err.Error())
		return
	}
	verb := utf("open")
	target := utf(path)
	r, _, _ := pShellExecuteW.Call(app.hwnd, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, SW_SHOWNORMAL)
	if r <= 32 {
		status("Could not open audio setup; run INSTALL_AUDIO_ENGINE.cmd in the ZIP.")
		return
	}
	status("Audio engine setup opened. Once installed, return here and choose your MP3.")
}
func chooseLyreFile() {
	buf := make([]uint16, 8192)
	filter := utfMultiSZ("MP4, Audio and MIDI\x00*.mp4;*.mkv;*.mov;*.webm;*.avi;*.mid;*.midi;*.mp3;*.wav;*.flac;*.ogg;*.m4a\x00Piano tutorial video\x00*.mp4;*.mkv;*.mov;*.webm;*.avi\x00MIDI files\x00*.mid;*.midi\x00Audio files\x00*.mp3;*.wav;*.flac;*.ogg;*.m4a\x00\x00")
	ofn := openFilenameW{LStructSize: uint32(unsafe.Sizeof(openFilenameW{})), HwndOwner: app.hwnd,
		Filter: &filter[0], File: &buf[0], MaxFile: uint32(len(buf)), Title: utf("Choose piano MP4, audio or MIDI to convert"), Flags: OFN_EXPLORER | OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST}
	r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		if code, _, _ := pCommDlgExtendedError.Call(); code != 0 {
			status(fmt.Sprintf("Import Song file picker failed (Windows error %d)", code))
		}
		return
	}
	path := syscallUTF16(buf)
	if path == "" {
		return
	}
	app.lyreSelectedPath = path
	clearLyrePrepared()
	setText(app.lyreSong, displayName(path))
	status("File selected. For MP4 select VISUAL / HYBRID / AUDIO. Then PREPARE PREVIEW.")
}
func syscallUTF16(buf []uint16) string {
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return syscall.UTF16ToString(buf[:n])
}
func lyreConvertSelected() {
	if app.lyreBusy {
		status("Conversion is already running. The app stays responsive.")
		return
	}
	path := app.lyreSelectedPath
	if path == "" {
		status("Click IMPORT SONG and choose an MP3 or MIDI file first.")
		return
	}
	title := strings.TrimSpace(getText(app.lyreSong))
	if title == "" {
		title = displayName(path)
	}
	skipMs, err := parseLyreStartTime(getText(app.lyreOffset))
	if err != nil {
		status("Start time: " + err.Error())
		return
	}
	speed, err := strconv.ParseFloat(strings.TrimSpace(getText(app.lyreSpeed)), 64)
	if err != nil || speed < 0.5 || speed > 2 {
		status("Speed must be between 0.5 and 2.0 (e.g. 1.0).")
		return
	}
	if !fileExists(path) {
		status("Selected song no longer exists. Re-import it.")
		return
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".mid" && ext != ".midi" && !isVideoPath(path) && findAudioEngine() == "" {
		status("MP3 transcription needs Basic Pitch. Click SETUP AUDIO ENGINE first.")
		return
	}
	arrangeMode := normalizeLyreArrangeMode(app.lyreArrangeMode)
	key := ""
	if arrangeMode == lyreArrangeCreative || arrangeMode == lyreArrangeAgent {
		key, err = loadEncryptedKey()
		if err != nil {
			status("AI Conversion needs your connected OpenAI API key. Open AI Studio to connect it: " + err.Error())
			return
		}
	}
	app.lyreBusy = true
	if arrangeMode == lyreArrangeCreative {
		status("Creative AI conversion started. First transcribing " + byteshort(title, 40) + ", then arranging note sections with OpenAI credits...")
	} else if arrangeMode == lyreArrangeAgent {
		status("Agent Pro started: transcribe, plan entire song, arrange and critique with GPT-6.1 Sol. Uses API credits...")
	} else {
		status("Preparing " + byteshort(title, 40) + " from " + formatLyreStartTime(skipMs) + " (" + fmt.Sprintf("%d", skipMs/1000) + " seconds) ...")
	}
	uiHWND := app.hwnd
	quality := app.lyreQuality // snapshot GUI state before starting worker
	videoMode := app.videoMode
	model := app.aiModel
	agentBudget := normalizeAgentBudget(app.agentBudgetCents)
	clearLyrePrepared()
	go func() {
		score, e := prepareLyreWithVideo(path, skipMs, speed, quality, videoMode)
		finalTitle := title
		if e == nil && (arrangeMode == lyreArrangeCreative || arrangeMode == lyreArrangeAgent) {
			if len(score.Events) < 4 {
				e = errors.New("the transcription contains too few notes for AI Arrangement")
			} else {
				initialEvents := len(score.Events)
				refs := lyreSourceBundle{Version: 1, SourceFile: path, StartMs: score.SourceStartMs, PlaybackSpeed: score.PlaybackSpeed, Transpose: score.SemitoneShift, PianoNotes: score.SourcePiano}
				report := func(msg string) {
					select {
					case creativeProgress <- msg:
					default:
					}
					pPostMessageW.Call(uiHWND, WM_APP+5, 0, 0)
				}
				report(fmt.Sprintf("Visual/audio transcription complete: %d events. Starting %s...", initialEvents, lyreArrangeDisplay(arrangeMode)))
				if arrangeMode == lyreArrangeAgent {
					score, e = createAgentProArrangement(key, title, score, refs, agentBudget, report)
				} else {
					score, e = createCreativeArrangement(key, model, title, "Create a complete, recognizable, smooth Genshin lyre version of this imported song. Full Arrangement is preferred; creatively repair false notes and rough chords rather than blindly following transcription.", score, refs, report)
				}
				if e == nil {
					finalTitle = title + " - " + lyreArrangeDisplay(arrangeMode)
					if verr := validateNonemptyAHK(generateLyreAHK(finalTitle, score)); verr != nil {
						e = fmt.Errorf("Creative AI generated unplayable output: %w", verr)
					}
				}
			}
		}
		lyreResults <- lyreResult{Title: finalTitle, Mode: arrangeMode, Score: score, Err: e}
		pPostMessageW.Call(uiHWND, WM_APP+3, 0, 0)
	}()
}
func prepareLyreWithVideo(path string, skipMs int64, speed float64, quality string, videoMode string) (lyreScore, error) {
	if isVideoPath(path) {
		return prepareVideoLyre(path, skipMs, speed, quality, videoMode)
	}
	return prepareLyre(path, skipMs, speed, quality)
}

func prepareLyre(path string, skipMs int64, speed float64, quality string) (lyreScore, error) {
	midiPath := path
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".mid" && ext != ".midi" {
		engine := findAudioEngine()
		if engine == "" {
			return lyreScore{}, errors.New("Basic Pitch is not installed; click SETUP AUDIO ENGINE")
		}
		outdir, err := os.MkdirTemp("", "cazy-lyre-audio-*")
		if err != nil {
			return lyreScore{}, err
		}
		defer os.RemoveAll(outdir)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, engine, outdir, path)
		cmd.Dir = outdir
		// Basic Pitch prints Unicode progress glyphs. On Windows a GUI-launched
		// Python process can default redirected stdout to the cp1252 ANSI page,
		// causing UnicodeEncodeError before any MIDI file is generated.
		// Give only this child process UTF-8 stdio; no system-wide changes.
		cmd.Env = append(cmd.Environ(),
			"PYTHONIOENCODING=utf-8",
			"PYTHONUTF8=1",
			"PYTHONUNBUFFERED=1",
			"NO_COLOR=1",
			"TF_CPP_MIN_LOG_LEVEL=2",
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			report := strings.TrimSpace(string(output))
			if len(report) > 1200 {
				report = report[len(report)-1200:]
			}
			if ctx.Err() != nil {
				return lyreScore{}, fmt.Errorf("audio transcription timed out: %w", ctx.Err())
			}
			if strings.Contains(report, "UnicodeEncodeError") || strings.Contains(report, "charmap") {
				return lyreScore{}, fmt.Errorf("Basic Pitch stopped while printing Unicode text. The app now forces UTF-8, but your installed audio engine still returned an encoding error: %w\n%s", err, report)
			}
			return lyreScore{}, fmt.Errorf("Basic Pitch failed: %w\n%s", err, report)
		}
		found := []string{}
		entries, err := os.ReadDir(outdir)
		if err != nil {
			return lyreScore{}, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			suffix := strings.ToLower(filepath.Ext(e.Name()))
			if suffix == ".mid" || suffix == ".midi" {
				found = append(found, filepath.Join(outdir, e.Name()))
			}
		}
		if len(found) == 0 {
			return lyreScore{}, errors.New("Audio engine finished but did not produce a MIDI file")
		}
		midiPath = found[0]
	}
	notes, err := parseMIDIFile(midiPath)
	if err != nil {
		return lyreScore{}, err
	}
	score, err := createLyreScoreQuality(notes, skipMs, speed, quality)
	if err != nil {
		return lyreScore{}, err
	}
	return score, nil
}

// Preview is generated in memory. Never silently overwrite or auto-run a macro.
func clearLyrePrepared() {
	stopLyrePreview()
	app.lyrePrepared = lyreScore{}
	app.lyrePreparedTitle = ""
	app.lyrePreparedMode = ""
}
func setLyreArrangeMode(mode string) {
	if app.lyreBusy {
		status("Finish the current conversion before changing arrangement mode.")
		return
	}
	mode = normalizeLyreArrangeMode(mode)
	if mode == app.lyreArrangeMode {
		return
	}
	app.lyreArrangeMode = mode
	save()
	clearLyrePrepared()
	switch mode {
	case lyreArrangeCreative:
		status("CREATIVE AI enabled: 1 arrangement pass per section. Uses OpenAI API credits.")
	case lyreArrangeAgent:
		status("AGENT PRO enabled: Sol Director → Arranger → Critic → selective polish. Preview before exporting. Uses credits.")
	default:
		status("STANDARD selected: local transcription and quality optimization, no API credits.")
	}
	redraw()
}
func setQuality(quality string) {
	quality = normalizeLyreQuality(quality)
	if quality == app.lyreQuality {
		return
	}
	app.lyreQuality = quality
	save()
	clearLyrePrepared()
	status("Quality set to " + qualityDisplay(quality) + ". Click PREPARE PREVIEW to rebuild the arrangement.")
}
func beginLyrePreview() {
	if len(app.lyrePrepared.Events) == 0 {
		status("Prepare a song first, then click PLAY PREVIEW.")
		return
	}
	stopLyrePreview()
	ctx, cancel := context.WithCancel(context.Background())
	app.lyrePreviewCancel = cancel
	app.lyrePreviewID++
	id := app.lyrePreviewID
	score := app.lyrePrepared
	hwnd := app.hwnd
	status("Playing Windows harp preview — this approximates, but does not exactly match, Genshin's lyre sound.")
	go func() {
		err := previewMusic(ctx, score)
		previewResults <- previewResult{ID: id, Err: err}
		pPostMessageW.Call(hwnd, WM_APP+4, 0, 0)
	}()
}
func stopLyrePreview() {
	stopSampledSound()
	if app.lyrePreviewCancel != nil {
		app.lyrePreviewCancel()
		app.lyrePreviewCancel = nil
	}
	app.lyrePreviewID++
}
func exportLyreNow() {
	if len(app.lyrePrepared.Events) == 0 {
		status("No prepared arrangement. Click PREPARE PREVIEW first.")
		return
	}
	if app.lyreBusy {
		status("Finish preparing the preview first.")
		return
	}
	name := app.lyrePreparedTitle
	if strings.TrimSpace(name) == "" {
		name = "Cazy Lyre Song"
	}
	outputDir := filepath.Join(filepath.Dir(appDataPath()), "Generated Macros")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		status(err.Error())
		return
	}
	safe := safeMacroName(name)
	filename := filepath.Join(outputDir, safe+"_Lyre.ahk")
	for i := 2; fileExists(filename); i++ {
		filename = filepath.Join(outputDir, fmt.Sprintf("%s_Lyre_%d.ahk", safe, i))
	}
	if err := os.WriteFile(filename, []byte(generateLyreAHK(name, app.lyrePrepared)), 0644); err != nil {
		status(err.Error())
		return
	}
	// Keep the original extracted MIDI note events as a local reference for Creative Arrangement.
	if err := saveSourceSidecar(filename, app.lyreSelectedPath, app.lyrePrepared); err != nil {
		status("AHK created, but source-note reference could not be saved: " + err.Error())
	}
	stopLyrePreview()
	addPaths([]string{filename})
	status(fmt.Sprintf("Exported %s | %s | %s | %d timed events to library.", name, lyreArrangeDisplay(app.lyrePreparedMode), qualityDisplay(app.lyrePrepared.Quality), len(app.lyrePrepared.Events)))
}

// Arrow controls adjust the start position without asking users to type
// an ambiguous decimal like 1.40. They never alter the musical contents.
func nudgeLyreOffset(seconds int64) {
	next, err := nudgeLyreStartTime(getText(app.lyreOffset), seconds)
	if err != nil {
		status("Start time: " + err.Error())
		return
	}
	setText(app.lyreOffset, next)
	clearLyrePrepared()
	startMs, _ := parseLyreStartTime(next)
	status(fmt.Sprintf("Song will start at %s (%d seconds). Click PREPARE PREVIEW.", next, startMs/1000))
}
func paintLyre(hdc uintptr, w, h int) {
	rounded(hdc, 283, 280, w-315, 131, palette.panel, palette.border, 16)
	textDraw(hdc, "STEP 1  ·  IMPORT PIANO MP4 / AUDIO / MIDI", 302, 284, w-354, 22, palette.accent2, app.microFont, DT_LEFT)
	button(hdc, "lyre-import", "+ IMPORT SONG / VIDEO", 304, 308, 184, 35, 1)
	button(hdc, "lyre-find", "FIND SONG BY NAME", 495, 308, 188, 35, 0)
	button(hdc, "lyre-setup", "SETUP AUDIO ENGINE", w-249, 308, 202, 35, 0)
	textDraw(hdc, "MP4 DETECTION:", 304, 350, 122, 25, palette.muted, app.microFont, DT_LEFT)
	videoChoices := []struct{ id, label, mode string }{{"video-visual", "AUTO VIS", videoVisual}, {"video-overlay", "LYRE KEYS", videoOverlay}, {"video-hybrid", "HYBRID", videoHybrid}, {"video-audio", "AUDIO", videoAudio}}
	for i, v := range videoChoices {
		st := 0
		if app.videoMode == v.mode {
			st = 1
		}
		button(hdc, v.id, v.label, 415+i*93, 348, 90, 28, st)
	}
	button(hdc, "lyre-setup-video", "SETUP VIDEO ENGINE", w-249, 348, 202, 29, 0)
	hint := "No song selected"
	if app.lyreSelectedPath != "" {
		hint = app.lyreSelectedPath
	}
	textDraw(hdc, byteshort(hint, 110), 304, 380, w-351, 23, palette.muted, app.subFont, DT_LEFT)

	rounded(hdc, 283, 417, w-315, 79, palette.panel, palette.border, 16)
	textDraw(hdc, "STEP 2  ·  LYRE QUALITY + ARRANGEMENT MODE", 302, 420, w-355, 21, palette.accent2, app.microFont, DT_LEFT)
	standardStyle, creativeStyle, proStyle := 0, 0, 0
	switch app.lyreArrangeMode {
	case lyreArrangeAgent:
		proStyle = 1
	case lyreArrangeCreative:
		creativeStyle = 1
	default:
		standardStyle = 1
	}
	button(hdc, "lyre-arrange-standard", "STANDARD", w-424, 418, 116, 26, standardStyle)
	button(hdc, "lyre-arrange-creative", "CREATIVE AI", w-302, 418, 128, 26, creativeStyle)
	button(hdc, "lyre-arrange-agent", "AGENT PRO", w-169, 418, 135, 26, proStyle)
	choices := []struct{ id, label, quality string }{{"lyre-quality-clean", "CLEAN MELODY", qualityClean}, {"lyre-quality-balanced", "BALANCED", qualityBalanced}, {"lyre-quality-full", "FULL ARRANGEMENT", qualityFull}}
	bw := (w - 375) / 3
	for i, c := range choices {
		style := 0
		if app.lyreQuality == c.quality {
			style = 1
		}
		button(hdc, c.id, c.label, 304+i*(bw+10), 448, bw, 33, style)
	}

	rounded(hdc, 283, 503, w-315, 99, palette.panel, palette.border, 16)
	step3 := "STEP 3  ·  ARRANGE AND PREPARE"
	if app.lyreArrangeMode == lyreArrangeCreative {
		step3 = "STEP 3  ·  CREATIVE AI (USES OPENAI CREDITS)"
	} else if app.lyreArrangeMode == lyreArrangeAgent {
		step3 = "STEP 3  ·  AGENT PRO (DIRECTOR / ARRANGER / CRITIC)"
	}
	textDraw(hdc, step3, 302, 506, 560, 20, palette.accent2, app.microFont, DT_LEFT)
	if app.lyreArrangeMode == lyreArrangeAgent {
		button(hdc, "lyre-agent-budget", fmt.Sprintf("BUDGET ~$%.2f", float64(normalizeAgentBudget(app.agentBudgetCents))/100), w-211, 504, 165, 25, 0)
	}
	textDraw(hdc, "Song name", 304, 532, 96, 25, palette.white, app.subFont, DT_LEFT)
	textDraw(hdc, "Speed", 718, 532, 85, 25, palette.white, app.subFont, DT_LEFT)
	textDraw(hdc, "Start at", 304, 565, 80, 26, palette.white, app.subFont, DT_LEFT)
	button(hdc, "lyre-minus-10", "◀ 10s", 487, 563, 62, 29, 0)
	button(hdc, "lyre-minus-1", "◀ 1s", 555, 563, 58, 29, 0)
	button(hdc, "lyre-plus-1", "1s ▶", 619, 563, 57, 29, 1)
	button(hdc, "lyre-plus-10", "10s ▶", 682, 563, 62, 29, 1)
	button(hdc, "lyre-plus-60", "+60s", 750, 563, 63, 29, 1)
	prepLabel := "PREPARE PREVIEW"
	if app.lyreArrangeMode == lyreArrangeCreative {
		prepLabel = "AI ARRANGE + PREVIEW"
	} else if app.lyreArrangeMode == lyreArrangeAgent {
		prepLabel = "AGENT PRO + PREVIEW"
	}
	button(hdc, "lyre-convert", prepLabel, w-251, 561, 200, 34, 2)

	rounded(hdc, 283, 609, w-315, 64, palette.panel, palette.border, 16)
	textDraw(hdc, "STEP 4  ·  LISTEN AND EXPORT", 302, 609, w-356, 22, palette.accent2, app.microFont, DT_LEFT)
	button(hdc, "lyre-preview", "▶ PLAY PREVIEW", 304, 634, 163, 31, 2)
	button(hdc, "lyre-preview-stop", "■ STOP", 473, 634, 104, 31, 3)
	button(hdc, "lyre-export", "EXPORT + ADD TO LIBRARY", 585, 634, 242, 31, 1)
	summary := "Prepare a song first"
	if len(app.lyrePrepared.Events) > 0 {
		summary = fmt.Sprintf("%s · %d events", lyreArrangeDisplay(app.lyrePreparedMode), len(app.lyrePrepared.Events))
	}
	_ = summary
	button(hdc, "lyre-preview-sampled", "REAL LYRE SOUND", 834, 634, maxInt(130, w-885), 31, 2)
}
func setLyreVisible() {
	for _, v := range []uintptr{app.lyreSong, app.lyreOffset, app.lyreSpeed} {
		if v == 0 {
			continue
		}
		if app.page == "lyre" {
			pShowWindow.Call(v, SW_SHOW)
		} else {
			pShowWindow.Call(v, SW_HIDE)
		}
	}
	if app.page == "lyre" {
		pMoveWindow.Call(app.lyreSong, 403, 531, 300, 27, 1)
		pMoveWindow.Call(app.lyreOffset, 384, 563, 97, 27, 1)
		pMoveWindow.Call(app.lyreSpeed, 789, 531, 79, 27, 1)
	}
}
