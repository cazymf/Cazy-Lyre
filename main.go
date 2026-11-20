//go:build windows

// Cazy Lyre v3.8 - integrated release updater - Agent Pro and one-time Windsong Lyre sample capture.
// No keyboard hooks are installed. ESC is never captured or remapped.
package main

import (
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	CW_USEDEFAULT        = 0x80000000
	WS_OVERLAPPED        = 0x00000000
	WS_CAPTION           = 0x00C00000
	WS_SYSMENU           = 0x00080000
	WS_MINIMIZEBOX       = 0x00020000
	WS_CHILD             = 0x40000000
	WS_VISIBLE           = 0x10000000
	WS_BORDER            = 0x00800000
	WS_VSCROLL           = 0x00200000
	ES_MULTILINE         = 0x0004
	ES_AUTOVSCROLL       = 0x0040
	ES_PASSWORD          = 0x0020
	ES_READONLY          = 0x0800
	ES_WANTRETURN        = 0x1000
	ES_AUTOHSCROLL       = 0x0080
	WM_CREATE            = 0x0001
	WM_DESTROY           = 0x0002
	WM_CLOSE             = 0x0010
	WM_PAINT             = 0x000F
	WM_ERASEBKGND        = 0x0014
	WM_COMMAND           = 0x0111
	WM_MOUSEMOVE         = 0x0200
	WM_LBUTTONDOWN       = 0x0201
	WM_LBUTTONDBLCLK     = 0x0203
	WM_MOUSEWHEEL        = 0x020A
	WM_DROPFILES         = 0x0233
	WM_KEYDOWN           = 0x0100
	WM_TIMER             = 0x0113
	WM_CTLCOLOREDIT      = 0x0133
	WM_CTLCOLORSTATIC    = 0x0138
	WM_APP               = 0x8000
	WM_SETFONT           = 0x0030
	WM_GETTEXTLENGTH     = 0x000E
	WM_GETTEXT           = 0x000D
	WM_SETTEXT           = 0x000C
	EM_SETSEL            = 0x00B1
	EM_SETCUEBANNER      = 0x1501
	EN_CHANGE            = 0x0300
	EN_KILLFOCUS         = 0x0200
	VK_F2                = 0x71
	VK_RETURN            = 0x0D
	VK_DELETE            = 0x2E
	DT_LEFT              = 0x0000
	DT_CENTER            = 0x0001
	DT_RIGHT             = 0x0002
	DT_VCENTER           = 0x0004
	DT_SINGLELINE        = 0x0020
	DT_END_ELLIPSIS      = 0x8000
	DT_NOPREFIX          = 0x0800
	TRANSPARENT          = 1
	PS_SOLID             = 0
	OFN_EXPLORER         = 0x00080000
	OFN_FILEMUSTEXIST    = 0x00001000
	OFN_PATHMUSTEXIST    = 0x00000800
	OFN_ALLOWMULTISELECT = 0x00000200
	SW_SHOW              = 5
	SW_HIDE              = 0
	SW_RESTORE           = 9
	SW_SHOWNORMAL        = 1
	MB_OK                = 0
	MB_ICONINFORMATION   = 0x40
	MB_ICONERROR         = 0x10
	GWLP_USERDATA        = -21
	CS_DBLCLKS           = 0x0008
	ID_EDIT_SEARCH       = 1001
	ID_EDIT_RENAME       = 1002
	ID_EDIT_API_KEY      = 1003
	ID_EDIT_AI_PROMPT    = 1004
	ID_EDIT_AI_OUTPUT    = 1005
	ID_EDIT_AI_FILENAME  = 1006
	ID_EDIT_LYRE_SONG    = 1007
	ID_EDIT_LYRE_SKIP    = 1008
	ID_EDIT_LYRE_SPEED   = 1009
	ID_EDIT_SONG_QUERY   = 1010
)

//go:embed MacroDeck_Icon.ico
var iconData []byte

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	k32      = syscall.NewLazyDLL("kernel32.dll")

	pCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	pRegisterClassExW         = user32.NewProc("RegisterClassExW")
	pCreateWindowExW          = user32.NewProc("CreateWindowExW")
	pDefWindowProcW           = user32.NewProc("DefWindowProcW")
	pShowWindow               = user32.NewProc("ShowWindow")
	pUpdateWindow             = user32.NewProc("UpdateWindow")
	pDestroyWindow            = user32.NewProc("DestroyWindow")
	pGetMessageW              = user32.NewProc("GetMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pPostMessageW             = user32.NewProc("PostMessageW")
	pBeginPaint               = user32.NewProc("BeginPaint")
	pEndPaint                 = user32.NewProc("EndPaint")
	pInvalidateRect           = user32.NewProc("InvalidateRect")
	pGetClientRect            = user32.NewProc("GetClientRect")
	pSetWindowTextW           = user32.NewProc("SetWindowTextW")
	pGetWindowTextW           = user32.NewProc("GetWindowTextW")
	pSendMessageW             = user32.NewProc("SendMessageW")
	pSetFocus                 = user32.NewProc("SetFocus")
	pSetTimer                 = user32.NewProc("SetTimer")
	pKillTimer                = user32.NewProc("KillTimer")
	pMoveWindow               = user32.NewProc("MoveWindow")
	pEnableWindow             = user32.NewProc("EnableWindow")
	pOpenClipboard            = user32.NewProc("OpenClipboard")
	pEmptyClipboard           = user32.NewProc("EmptyClipboard")
	pSetClipboardData         = user32.NewProc("SetClipboardData")
	pCloseClipboard           = user32.NewProc("CloseClipboard")
	pGlobalAlloc              = k32.NewProc("GlobalAlloc")
	pGlobalLock               = k32.NewProc("GlobalLock")
	pGlobalUnlock             = k32.NewProc("GlobalUnlock")
	pGlobalFree               = k32.NewProc("GlobalFree")
	pCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap   = gdi32.NewProc("CreateCompatibleBitmap")
	pBitBlt                   = gdi32.NewProc("BitBlt")
	pDeleteDC                 = gdi32.NewProc("DeleteDC")
	pSetBkMode                = gdi32.NewProc("SetBkMode")
	pSetBkColor               = gdi32.NewProc("SetBkColor")
	pSetTextColor             = gdi32.NewProc("SetTextColor")
	pSelectObject             = gdi32.NewProc("SelectObject")
	pCreateSolidBrush         = gdi32.NewProc("CreateSolidBrush")
	pCreatePen                = gdi32.NewProc("CreatePen")
	pDeleteObject             = gdi32.NewProc("DeleteObject")
	pFillRect                 = user32.NewProc("FillRect")
	pRoundRect                = gdi32.NewProc("RoundRect")
	pEllipse                  = gdi32.NewProc("Ellipse")
	pDrawTextW                = user32.NewProc("DrawTextW")
	pCreateFontW              = gdi32.NewProc("CreateFontW")
	pSetWindowPos             = user32.NewProc("SetWindowPos")
	pGetOpenFileNameW         = comdlg32.NewProc("GetOpenFileNameW")
	pCommDlgExtendedError     = comdlg32.NewProc("CommDlgExtendedError")
	pDragAcceptFiles          = shell32.NewProc("DragAcceptFiles")
	pDragQueryFileW           = shell32.NewProc("DragQueryFileW")
	pDragFinish               = shell32.NewProc("DragFinish")
	pMessageBoxW              = user32.NewProc("MessageBoxW")
	pShellExecuteW            = shell32.NewProc("ShellExecuteW")
	pIsUserAnAdmin            = shell32.NewProc("IsUserAnAdmin")
	pSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	pGetModuleHandleW         = k32.NewProc("GetModuleHandleW")
	pLoadCursorW              = user32.NewProc("LoadCursorW")
	pDwmSetWindowAttribute    = dwmapi.NewProc("DwmSetWindowAttribute")
)

type rect struct{ L, T, R, B int32 }
type wndClassEx struct {
	CbSize, Style                      uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSmall                          uintptr
}
type point struct{ X, Y int32 }
type msg struct {
	HWnd           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             point
	LPrivate       uint32
}
type paintStruct struct {
	HDC                uintptr
	Erase              int32
	Paint              rect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type openFilenameW struct {
	LStructSize                uint32
	HwndOwner, HInstance       uintptr
	Filter, CustomFilter       *uint16
	MaxCustFilter, FilterIndex uint32
	File                       *uint16
	MaxFile                    uint32
	FileTitle                  *uint16
	MaxFileTitle               uint32
	InitialDir, Title          *uint16
	Flags                      uint32
	FileOffset, FileExtension  uint16
	DefExt                     *uint16
	CustData, Hook             uintptr
	TemplateName               *uint16
	Reserved                   uintptr
	DwReserved, FlagsEx        uint32
}

type macro struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Favorite bool   `json:"favorite"`
	Added    int64  `json:"added"`
}
type config struct {
	Version          int     `json:"version"`
	Interpreter      string  `json:"interpreter,omitempty"`
	Mode             string  `json:"mode,omitempty"`
	AIModel          string  `json:"ai_model,omitempty"`
	LyreQuality      string  `json:"lyre_quality,omitempty"`
	LyreArrangeMode  string  `json:"lyre_arrange_mode,omitempty"`
	AgentBudgetCents int     `json:"agent_budget_cents,omitempty"`
	VideoMode        string  `json:"video_mode,omitempty"`
	AIEditMode       string  `json:"ai_edit_mode,omitempty"`
	Macros           []macro `json:"macros"`
}
type runningSession struct {
	Cmd     *exec.Cmd
	Path    string
	Name    string
	PID     int
	Started time.Time
}
type hitBox struct {
	ID         string
	X, Y, W, H int
}

var app struct {
	hwnd, search, rename, keyEdit, aiPrompt, aiOutput, aiFileName, urlEdit uintptr
	itemFont, headingFont, titleFont, subFont, microFont, brandFont        uintptr
	editBrush                                                              uintptr
	brush                                                                  map[uint32]uintptr
	pen                                                                    map[uint32]uintptr
	hits                                                                   []hitBox
	selected                                                               string
	page                                                                   string
	query                                                                  string
	hovered                                                                string
	rows                                                                   []int
	items                                                                  []macro
	scroll                                                                 int
	w, h                                                                   int
	status                                                                 string
	cfgPath, interpreter                                                   string
	detectedAHK                                                            string
	mode                                                                   string                     // "single" or "multi"; persisted in library.json
	sessions                                                               map[string]*runningSession // currently tracked processes, keyed by normalized path
	lastStarted                                                            string
	aiModel                                                                string
	aiEditMode                                                             string
	aiCreativeScore                                                        lyreScore
	aiCreativeOriginalPath                                                 string
	aiBusy                                                                 bool
	aiKeyPresent                                                           bool
	aiLastGenerated                                                        string
	aiLastKind                                                             string
	aiRequestStarted                                                       time.Time
	renamePath                                                             string
	lyreSong, lyreOffset, lyreSpeed, songQuery                             uintptr
	songSearchBusy                                                         bool
	songSearchSummary                                                      string
	songSearchSources                                                      []songSearchSource
	lyreSelectedPath                                                       string
	lyreBusy                                                               bool
	lyreQuality                                                            string
	lyreArrangeMode                                                        string
	agentBudgetCents                                                       int
	lyrePreparedMode                                                       string
	videoMode                                                              string
	lyrePrepared                                                           lyreScore
	lyrePreparedTitle                                                      string
	lyrePreviewCancel                                                      context.CancelFunc
	captureCancel                                                          context.CancelFunc
	captureBusy                                                            bool
	captureProgress                                                        int
	captureLastKey                                                         string
	captureState                                                           string
	lyrePreviewID                                                          uint64
	tempoSpeed                                                             float64
}

// Async API responses are delivered to the GUI thread through a Windows message.
// The network worker never reads or modifies GUI state directly.
type aiResult struct {
	Text              string
	Err               error
	Kind              string
	MusicBlocks       []preservedMusicBlock
	OriginalNoteCount int
	CreativeScore     lyreScore
}

var aiResults = make(chan aiResult, 2)
var creativeProgress = make(chan string, 40)

var palette = struct{ bg, sidebar, panel, panelHover, hero, border, line, white, muted, subtle, accent, accent2, green, red, edit uint32 }{
	bg: 0x0C1020, sidebar: 0x111629, panel: 0x181F35, panelHover: 0x202943,
	hero: 0x1C2145, border: 0x303A55, line: 0x273049,
	white: 0xF4F7FF, muted: 0xA5B0CE, subtle: 0x71809E,
	accent: 0x7664FF, accent2: 0x35C4E4, green: 0x47D8B1, red: 0xF17991, edit: 0x141B30,
}

func colorref(hex uint32) uint32 { return (hex>>16)&0xff | (hex & 0xff00) | (hex&0xff)<<16 }
func utf(s string) *uint16       { return syscall.StringToUTF16Ptr(s) }
func utfarr(s string) []uint16   { return syscall.StringToUTF16(s) }

// File dialog filters are MULTI_SZ sequences with embedded NUL separators.
// syscall.StringToUTF16 rejects interior NULs and caused Import Song to panic.
func utfMultiSZ(s string) []uint16 { return utf16.Encode([]rune(s)) }
func loword(v uintptr) int         { return int(v & 0xFFFF) }
func hiword(v uintptr) int         { return int((v >> 16) & 0xFFFF) }
func signedword(v uintptr) int     { return int(int16(v & 0xFFFF)) }
func byteshort(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 2 {
		return s
	}
	return string(r[:max-1]) + "…"
}

func appDataPath() string {
	d := os.Getenv("LOCALAPPDATA")
	if d == "" {
		d = os.Getenv("APPDATA")
	}
	if d == "" {
		d = "."
	}
	return filepath.Join(d, "MacroDeck Pro", "library.json")
}
func save() {
	dir := filepath.Dir(app.cfgPath)
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(config{Version: 7, Interpreter: app.interpreter, Mode: app.mode, AIModel: app.aiModel, LyreQuality: app.lyreQuality, LyreArrangeMode: app.lyreArrangeMode, AgentBudgetCents: normalizeAgentBudget(app.agentBudgetCents), VideoMode: app.videoMode, AIEditMode: app.aiEditMode, Macros: app.items}, "", "  ")
	if err != nil {
		return
	}
	tmp := app.cfgPath + ".tmp"
	if os.WriteFile(tmp, data, 0644) == nil {
		_ = os.Rename(tmp, app.cfgPath)
	}
}
func load() {
	app.cfgPath = appDataPath()
	data, err := os.ReadFile(app.cfgPath)
	if err == nil {
		var cfg config
		if json.Unmarshal(data, &cfg) == nil {
			app.items = cfg.Macros
			app.interpreter = cfg.Interpreter
			app.mode = normalizeMode(cfg.Mode) // old v1.1 libraries default to Single Mode
			app.aiModel = normalizeAIModel(cfg.AIModel)
			app.lyreQuality = normalizeLyreQuality(cfg.LyreQuality)
			app.lyreArrangeMode = normalizeLyreArrangeMode(cfg.LyreArrangeMode)
			app.agentBudgetCents = normalizeAgentBudget(cfg.AgentBudgetCents)
			app.videoMode = normalizeVideoMode(cfg.VideoMode)
			app.aiEditMode = normalizeAIEditMode(cfg.AIEditMode)
			app.detectedAHK = findAHK()
			app.aiKeyPresent = hasEncryptedKey()
			return
		}
	}
	// Carry over existing MacroDeck AHK library automatically on first run.
	ini := filepath.Join(os.Getenv("APPDATA"), "MacroDeck", "library.ini")
	old, err := os.ReadFile(ini)
	if err == nil {
		for _, line := range strings.Split(strings.ReplaceAll(string(old), "\r", ""), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(strings.ToLower(line), "path") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) < 2 {
				continue
			}
			p := strings.TrimSpace(parts[1])
			if extOK(p) && !hasItem(p) {
				app.items = append(app.items, macro{Name: displayName(p), Path: p, Added: time.Now().Unix()})
			}
		}
	}
	app.detectedAHK = findAHK()
	app.aiModel = normalizeAIModel(app.aiModel)
	app.lyreArrangeMode = normalizeLyreArrangeMode(app.lyreArrangeMode)
	app.agentBudgetCents = normalizeAgentBudget(app.agentBudgetCents)
	app.lyreQuality = normalizeLyreQuality(app.lyreQuality)
	app.aiKeyPresent = hasEncryptedKey()
	save()
}
func extOK(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".ahk" || ext == ".exe"
}
func hasItem(p string) bool {
	for _, m := range app.items {
		if strings.EqualFold(m.Path, p) {
			return true
		}
	}
	return false
}
func displayName(p string) string {
	n := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	n = strings.ReplaceAll(n, "_", " ")
	return n
}
func status(s string) { app.status = s; redraw() }
func redraw() {
	if app.hwnd != 0 {
		pInvalidateRect.Call(app.hwnd, 0, 0)
	}
}
func msgBox(title, body string, flags uintptr) {
	pMessageBoxW.Call(app.hwnd, uintptr(unsafe.Pointer(utf(body))), uintptr(unsafe.Pointer(utf(title))), flags)
}
func getText(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	sz, _, _ := pSendMessageW.Call(hwnd, WM_GETTEXTLENGTH, 0, 0)
	buff := make([]uint16, int(sz)+2)
	pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buff[0])), uintptr(len(buff)))
	return syscall.UTF16ToString(buff)
}
func setText(hwnd uintptr, s string) { pSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(utf(s)))) }
func selectedIndex() int {
	for i, m := range app.items {
		if strings.EqualFold(m.Path, app.selected) {
			return i
		}
	}
	return -1
}
func normalizeMode(mode string) string {
	if mode == "multi" {
		return "multi"
	}
	return "single"
}
func sessionKey(path string) string { return strings.ToLower(filepath.Clean(path)) }
func isRunning(path string) bool    { _, ok := app.sessions[sessionKey(path)]; return ok }
func active() bool                  { return len(app.sessions) > 0 }
func activeCount() int              { return len(app.sessions) }
func latestSession() *runningSession {
	var latest *runningSession
	for _, session := range app.sessions {
		if latest == nil || session.Started.After(latest.Started) {
			latest = session
		}
	}
	return latest
}
func stopSession(path string) bool {
	key := sessionKey(path)
	session, ok := app.sessions[key]
	if !ok {
		return false
	}
	delete(app.sessions, key)
	if session.Cmd != nil && session.Cmd.Process != nil {
		_ = session.Cmd.Process.Kill() // Windows TerminateProcess; Wait() is handled asynchronously
	}
	return true
}
func stopAll(notice bool) {
	count := len(app.sessions)
	for key, session := range app.sessions {
		if session.Cmd != nil && session.Cmd.Process != nil {
			_ = session.Cmd.Process.Kill()
		}
		delete(app.sessions, key)
	}
	app.lastStarted = ""
	if notice {
		if count == 0 {
			status("No macros are currently running.")
		} else {
			status(fmt.Sprintf("Stopped all %d macro(s).", count))
		}
	} else {
		redraw()
	}
}
func stopSelected() {
	if app.selected != "" && stopSession(app.selected) {
		status("Stopped selected macro. Other running macros are unchanged.")
		return
	}
	// In Single Mode the Stop button works even when no row is selected.
	if activeCount() == 1 {
		for _, session := range app.sessions {
			stopSession(session.Path)
			break
		}
		status("Macro stopped. Ready for another.")
		return
	}
	if activeCount() == 0 {
		status("No macro is currently running.")
	} else {
		status("Select a running macro to stop, or click STOP ALL.")
	}
}
func setMode(mode string) {
	mode = normalizeMode(mode)
	if app.mode == mode {
		return
	}
	app.mode = mode
	if mode == "single" && activeCount() > 1 {
		// Keep the newest started macro when switching out of Multi Mode.
		last := latestSession()
		for key, sess := range app.sessions {
			if last == nil || sess.PID != last.PID {
				if sess.Cmd != nil && sess.Cmd.Process != nil {
					_ = sess.Cmd.Process.Kill()
				}
				delete(app.sessions, key)
			}
		}
		if last != nil {
			app.lastStarted = last.Path
		}
	}
	save()
	if mode == "single" {
		status("Single Mode on: launching another macro closes the previous one.")
	} else {
		status("Multi Mode on: multiple different macros can run together.")
	}
}

func runSelected() {
	idx := selectedIndex()
	if idx < 0 {
		status("Choose a macro in the list first.")
		return
	}
	m := app.items[idx]
	if st, err := os.Stat(m.Path); err != nil || st.IsDir() {
		status("File missing. Re-import it from its new location.")
		return
	}
	var cmd *exec.Cmd
	if strings.EqualFold(filepath.Ext(m.Path), ".ahk") {
		interpreter := findAHK()
		if interpreter == "" {
			msgBox("AutoHotkey v2 not found", "Choose the AutoHotkey v2 executable under Settings, or install AutoHotkey v2 first.", MB_ICONERROR)
			status("AutoHotkey v2 interpreter not found.")
			return
		}
		cmd = exec.Command(interpreter, m.Path)
	} else {
		cmd = exec.Command(m.Path)
	}
	cmd.Dir = filepath.Dir(m.Path)
	// First validate the new command so a missing AHK interpreter does not
	// accidentally stop the macro already playing.
	// Single Mode then stops everything; Multi Mode restarts only this file.
	if app.mode == "single" {
		stopAll(false)
	} else {
		stopSession(m.Path)
	}
	if err := cmd.Start(); err != nil {
		status("Could not start: " + err.Error())
		return
	}
	pid := cmd.Process.Pid
	app.sessions[sessionKey(m.Path)] = &runningSession{Cmd: cmd, Path: m.Path, Name: m.Name, PID: pid, Started: time.Now()}
	app.lastStarted = m.Path
	status(fmt.Sprintf("Running %s  ·  %d active  ·  %s Mode", m.Name, activeCount(), strings.Title(app.mode)))
	// Post only the process ID; all GUI state stays on the locked Win32 UI thread.
	go func() { _ = cmd.Wait(); pPostMessageW.Call(app.hwnd, WM_APP+1, uintptr(pid), 0) }()
}
func findAHK() string {
	if app.interpreter != "" {
		if _, err := os.Stat(app.interpreter); err == nil {
			return app.interpreter
		}
	}
	roots := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")}
	suffixes := []string{
		"AutoHotkey\\v2\\AutoHotkey64.exe", "AutoHotkey\\v2\\AutoHotkey32.exe",
		"AutoHotkey\\AutoHotkey64.exe", "AutoHotkey\\AutoHotkey.exe",
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, s := range suffixes {
			p := filepath.Join(root, s)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	for _, exe := range []string{"AutoHotkey64.exe", "AutoHotkey.exe", "AutoHotkey32.exe"} {
		if p, err := exec.LookPath(exe); err == nil {
			return p
		}
	}
	return ""
}
func addPaths(paths []string) {
	count := 0
	for _, p := range paths {
		p = filepath.Clean(p)
		if !extOK(p) || hasItem(p) {
			continue
		}
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			continue
		}
		app.items = append(app.items, macro{Name: displayName(p), Path: p, Added: time.Now().Unix()})
		app.selected = p
		count++
	}
	if count > 0 {
		save()
		// Import also occurs asynchronously after Lyre Converter finishes.
		// Always hide the previous page's native EDIT controls before painting
		// the library; otherwise converter values overlay the macro list.
		switchPage("library")
		status(fmt.Sprintf("Imported %d macro(s). Double-click one to launch it.", count))
	} else {
		status("No new .ahk or .exe files were added.")
	}
}
func removeSelected() {
	idx := selectedIndex()
	if idx < 0 {
		status("Select a macro to remove from the library.")
		return
	}
	stopSession(app.items[idx].Path)
	name := app.items[idx].Name
	app.items = append(app.items[:idx], app.items[idx+1:]...)
	app.selected = ""
	app.scroll = 0
	save()
	status("Removed " + name + " from library. Original file untouched.")
}
func toggleFavorite(path string) {
	for i := range app.items {
		if strings.EqualFold(app.items[i].Path, path) {
			app.items[i].Favorite = !app.items[i].Favorite
			save()
			break
		}
	}
	redraw()
}
func filterRows() {
	app.rows = app.rows[:0]
	q := strings.ToLower(strings.TrimSpace(app.query))
	for i, m := range app.items {
		if app.page == "favorites" && !m.Favorite {
			continue
		}
		if app.page == "running" && !isRunning(m.Path) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(m.Name+" "+m.Path), q) {
			continue
		}
		app.rows = append(app.rows, i)
	}
	sort.SliceStable(app.rows, func(i, j int) bool {
		a := app.items[app.rows[i]]
		b := app.items[app.rows[j]]
		if a.Favorite != b.Favorite {
			return a.Favorite
		}
		return a.Added > b.Added
	})
	if app.scroll > len(app.rows)-1 {
		app.scroll = len(app.rows) - 1
	}
	if app.scroll < 0 {
		app.scroll = 0
	}
}

func makeBrush(color uint32) uintptr {
	if b, ok := app.brush[color]; ok {
		return b
	}
	b, _, _ := pCreateSolidBrush.Call(uintptr(colorref(color)))
	app.brush[color] = b
	return b
}
func makePen(color uint32) uintptr {
	if p, ok := app.pen[color]; ok {
		return p
	}
	p, _, _ := pCreatePen.Call(PS_SOLID, 1, uintptr(colorref(color)))
	app.pen[color] = p
	return p
}
func fill(hdc uintptr, x, y, w, h int, color uint32) {
	r := rect{int32(x), int32(y), int32(x + w), int32(y + h)}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), makeBrush(color))
}
func rounded(hdc uintptr, x, y, w, h int, color, border uint32, radius int) {
	oldb, _, _ := pSelectObject.Call(hdc, makeBrush(color))
	oldp, _, _ := pSelectObject.Call(hdc, makePen(border))
	pRoundRect.Call(hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), uintptr(radius), uintptr(radius))
	pSelectObject.Call(hdc, oldb)
	pSelectObject.Call(hdc, oldp)
}
func dot(hdc uintptr, x, y, r int, color uint32) {
	oldb, _, _ := pSelectObject.Call(hdc, makeBrush(color))
	oldp, _, _ := pSelectObject.Call(hdc, makePen(color))
	pEllipse.Call(hdc, uintptr(x-r), uintptr(y-r), uintptr(x+r), uintptr(y+r))
	pSelectObject.Call(hdc, oldb)
	pSelectObject.Call(hdc, oldp)
}
func textDraw(hdc uintptr, s string, x, y, w, h int, col uint32, font uintptr, align uint32) {
	if s == "" {
		return
	}
	old, _, _ := pSelectObject.Call(hdc, font)
	pSetBkMode.Call(hdc, TRANSPARENT)
	pSetTextColor.Call(hdc, uintptr(colorref(col)))
	r := rect{int32(x), int32(y), int32(x + w), int32(y + h)}
	t := utfarr(s)
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), uintptr(align|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS|DT_NOPREFIX))
	pSelectObject.Call(hdc, old)
}
func hit(id string, x, y, w, h int) {
	app.hits = append(app.hits, hitBox{ID: id, X: x, Y: y, W: w, H: h})
}
func button(hdc uintptr, id, title string, x, y, w, h int, style int) {
	bg := palette.panel
	bord := palette.border
	fg := palette.white
	if style == 1 {
		bg = palette.accent
		bord = palette.accent
	}
	if style == 2 {
		bg = 0x25354D
		bord = 0x314661
		fg = palette.green
	}
	if style == 3 {
		bg = 0x342638
		bord = 0x553746
		fg = palette.red
	}
	if app.hovered == id {
		if style == 1 {
			bg = 0x8B79FF
		} else {
			bg = palette.panelHover
		}
	}
	rounded(hdc, x, y, w, h, bg, bord, 13)
	textDraw(hdc, title, x+9, y, w-18, h, fg, app.itemFont, DT_CENTER)
	hit(id, x, y, w, h)
}
func nav(hdc uintptr, id, label, sub string, y int, selected bool) {
	x := 18
	w := 219
	h := 47
	if selected {
		rounded(hdc, x, y, w, h, 0x2A2854, 0x3E3A77, 12)
		fill(hdc, x, y+9, 3, h-18, palette.accent)
	}
	color := palette.muted
	if selected {
		color = palette.white
	}
	textDraw(hdc, label, x+19, y+1, 185, 27, color, app.itemFont, DT_LEFT)
	textDraw(hdc, sub, x+19, y+25, 185, 15, palette.subtle, app.microFont, DT_LEFT)
	hit("nav:"+id, x, y, w, h)
}
func labelCount(hdc uintptr, n int, x, y int) {
	bg := palette.accent
	rounded(hdc, x, y, 35, 24, bg, bg, 10)
	textDraw(hdc, strconv.Itoa(n), x, y, 35, 24, palette.white, app.microFont, DT_CENTER)
}
func selectedRowRect(i int) (int, int, int, int) {
	return 283, 358 + (i-app.scroll)*63, app.w - 315, 56
}
func updateEditorVisibility() {
	if app.search == 0 {
		return
	}
	if app.page == "settings" || app.page == "ai" || app.page == "lyre" || app.page == "songsearch" || app.page == "instrument" || app.page == "tempo" || app.page == "updates" {
		pShowWindow.Call(app.search, SW_HIDE)
	} else {
		pShowWindow.Call(app.search, SW_SHOW)
		pMoveWindow.Call(app.search, 324, 315, uintptr(app.w-585), 26, 1)
	}
	if app.keyEdit != 0 {
		if app.page == "ai" {
			pShowWindow.Call(app.keyEdit, SW_SHOW)
			pShowWindow.Call(app.aiPrompt, SW_SHOW)
			pShowWindow.Call(app.aiOutput, SW_SHOW)
			pShowWindow.Call(app.aiFileName, SW_SHOW)
			aiLayout()
		} else {
			pShowWindow.Call(app.keyEdit, SW_HIDE)
			pShowWindow.Call(app.aiPrompt, SW_HIDE)
			pShowWindow.Call(app.aiOutput, SW_HIDE)
			pShowWindow.Call(app.aiFileName, SW_HIDE)
		}
	}
	if app.urlEdit != 0 {
		if app.page == "updates" {
			pShowWindow.Call(app.urlEdit, SW_SHOW)
			pMoveWindow.Call(app.urlEdit, 314, 470, uintptr(maxInt(160, app.w-390)), 28, 1)
		} else {
			pShowWindow.Call(app.urlEdit, SW_HIDE)
		}
	}
	setLyreVisible()
	if app.songQuery != 0 {
		if app.page == "songsearch" {
			pShowWindow.Call(app.songQuery, SW_SHOW)
			pMoveWindow.Call(app.songQuery, 471, 318, uintptr(maxInt(140, app.w-744)), 29, 1)
		} else {
			pShowWindow.Call(app.songQuery, SW_HIDE)
		}
	}
}
func aiLayout() {
	if app.keyEdit == 0 || app.w == 0 {
		return
	}
	contentRight := app.w - 31
	colW := (contentRight - 283 - 16) / 2
	x2 := 283 + colW + 16
	pMoveWindow.Call(app.keyEdit, 434, 307, uintptr(x2-452), 26, 1)
	pMoveWindow.Call(app.aiPrompt, 299, 466, uintptr(colW-32), uintptr(maxInt(80, app.h-617)), 1)
	pMoveWindow.Call(app.aiOutput, uintptr(x2+16), 466, uintptr(colW-32), uintptr(maxInt(80, app.h-617)), 1)
	pMoveWindow.Call(app.aiFileName, uintptr(x2+16), uintptr(app.h-133), uintptr(colW-200), 24, 1)
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var cr rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
	app.w = int(cr.R)
	app.h = int(cr.B)
	w, h := app.w, app.h
	if w <= 0 || h <= 0 {
		return
	}
	realDC := hdc
	memDC, _, _ := pCreateCompatibleDC.Call(realDC)
	bmp, _, _ := pCreateCompatibleBitmap.Call(realDC, uintptr(w), uintptr(h))
	if memDC == 0 || bmp == 0 {
		if bmp != 0 {
			pDeleteObject.Call(bmp)
		}
		if memDC != 0 {
			pDeleteDC.Call(memDC)
		}
		return
	}
	oldBitmap, _, _ := pSelectObject.Call(memDC, bmp)
	hdc = memDC
	app.hits = app.hits[:0]
	filterRows()
	fill(hdc, 0, 0, w, h, palette.bg)
	fill(hdc, 0, 0, 256, h, palette.sidebar)
	fill(hdc, 255, 0, 1, h, palette.border)
	// Sidebar brand block.
	rounded(hdc, 22, 28, 46, 46, palette.accent, palette.accent, 14)
	textDraw(hdc, "C", 22, 27, 46, 47, palette.white, app.titleFont, DT_CENTER)
	textDraw(hdc, "CAZY LYRE", 81, 25, 162, 31, palette.white, app.brandFont, DT_LEFT)
	textDraw(hdc, "AI  +  MACRO MANAGER", 82, 54, 168, 15, palette.subtle, app.microFont, DT_LEFT)
	fill(hdc, 21, 100, 213, 1, palette.line)
	textDraw(hdc, "WORKSPACE", 25, 125, 200, 24, palette.subtle, app.microFont, DT_LEFT)
	nav(hdc, "library", "Macro Library", "All imported scripts", 155, app.page == "library")
	nav(hdc, "favorites", "Favorites", "Pinned quick access", 204, app.page == "favorites")
	nav(hdc, "running", "Now Running", "Active macro session", 253, app.page == "running")
	nav(hdc, "ai", "AI Studio", "Generate and fix macros", 302, app.page == "ai")
	nav(hdc, "lyre", "Lyre Converter", "MP4 / MP3 / MIDI to AHK", 351, app.page == "lyre")
	nav(hdc, "instrument", "Lyre Capture", "One-time sound calibration", 400, app.page == "instrument")
	nav(hdc, "updates", "Updates", "Check, install, restart", 449, app.page == "updates")
	nav(hdc, "settings", "Settings", "Interpreter + preferences", 498, app.page == "settings")
	// Bottom account-like tile.
	rounded(hdc, 18, h-144, 218, 104, palette.panel, palette.border, 15)
	dot(hdc, 40, h-118, 5, palette.green)
	textDraw(hdc, "ADMIN SESSION ACTIVE", 55, h-132, 162, 27, palette.white, app.microFont, DT_LEFT)
	textDraw(hdc, "Elevated macro launcher", 34, h-97, 181, 20, palette.muted, app.subFont, DT_LEFT)
	textDraw(hdc, "ESC stays untouched", 34, h-75, 181, 20, palette.green, app.subFont, DT_LEFT)
	textDraw(hdc, "v3.11.0   ·   ADMIN MODE", 22, h-28, 210, 15, palette.subtle, app.microFont, DT_LEFT)
	// Header
	textDraw(hdc, "CONTROL CENTER", 284, 29, 450, 25, palette.accent2, app.microFont, DT_LEFT)
	title := "Your macros, organized."
	if app.page == "favorites" {
		title = "Your favorites."
	}
	if app.page == "running" {
		title = "Now running."
	}
	if app.page == "updates" {
		title = "Updates."
	}
	if app.page == "settings" {
		title = "Settings."
	}
	if app.page == "ai" {
		title = "AI Studio."
	}
	if app.page == "songsearch" {
		title = "Find Song Online."
	}
	if app.page == "instrument" {
		title = "Windsong Lyre Capture."
	}
	if app.page == "lyre" {
		title = "Lyre Converter."
	}
	if app.page == "tempo" {
		title = "Tempo Lab."
	}
	if app.page == "timeline" {
		title = "Timeline Lab."
	}
	textDraw(hdc, title, 282, 53, w-550, 47, palette.white, app.titleFont, DT_LEFT)
	subtitle := "A dedicated home for every script in your setup."
	if app.page == "updates" {
		subtitle = "Install verified releases without losing your songs or settings."
	}
	if app.page == "ai" {
		subtitle = "Generate AutoHotkey v2 scripts with your OpenAI API credits."
	}
	if app.page == "songsearch" {
		subtitle = "Search for real, cited MIDI and piano tutorial sources using OpenAI."
	}
	if app.page == "instrument" {
		subtitle = "Capture all 21 real Genshin notes once for an accurate virtual instrument."
	}
	if app.page == "lyre" {
		subtitle = "Extract MP4 piano notes visually, preview your lyre arrangement, then export."
	}
	if app.page == "tempo" {
		subtitle = "Speed up already-created AHK songs without losing your original."
	}
	if app.page == "timeline" {
		subtitle = "Trim intros, select where playback starts and ends, remove sections; keep your original."
	}
	textDraw(hdc, subtitle, 284, 97, w-320, 21, palette.muted, app.subFont, DT_LEFT)
	button(hdc, "import", "+  IMPORT MACROS", w-205, 50, 173, 40, 1)
	// Hero
	rounded(hdc, 282, 139, w-313, 133, palette.hero, 0x363465, 21)
	fill(hdc, 301, 161, 4, 87, palette.accent)
	textDraw(hdc, "ACTIVE SESSION", 322, 154, 310, 27, palette.accent2, app.microFont, DT_LEFT)
	runningText := "No macro is running"
	if activeCount() == 1 {
		if s := latestSession(); s != nil {
			runningText = s.Name
		}
	}
	if activeCount() > 1 {
		runningText = fmt.Sprintf("%d macros running", activeCount())
	}
	textDraw(hdc, runningText, 321, 179, w-797, 37, palette.white, app.headingFont, DT_LEFT)
	hint := "Select a macro and click Launch (or double-click it)."
	if active() {
		hint = fmt.Sprintf("%d active · %s Mode · individual macro hotkeys still work", activeCount(), strings.Title(app.mode))
	}
	textDraw(hdc, hint, 322, 215, w-770, 27, palette.muted, app.subFont, DT_LEFT)
	// Persistent two-button switch, visible on every page.
	singleStyle, multiStyle := 0, 0
	if app.mode == "single" {
		singleStyle = 1
	} else {
		multiStyle = 1
	}
	button(hdc, "mode-single", "SINGLE MODE", w-411, 151, 174, 32, singleStyle)
	button(hdc, "mode-multi", "MULTI MODE", w-228, 151, 174, 32, multiStyle)
	button(hdc, "run", "▶ LAUNCH", w-457, 194, 126, 42, 2)
	button(hdc, "stop", "■ STOP", w-322, 194, 149, 42, 3)
	button(hdc, "stopall", "STOP ALL", w-163, 194, 125, 42, 3)
	if app.page == "updates" {
		paintUpdates(hdc, w, h)
	} else if app.page == "settings" {
		paintSettings(hdc, w, h)
	} else if app.page == "ai" {
		paintAI(hdc, w, h)
	} else if app.page == "instrument" {
		paintInstrument(hdc, w, h)
	} else if app.page == "lyre" {
		paintLyre(hdc, w, h)
	} else if app.page == "songsearch" {
		paintSongSearch(hdc, w, h)
	} else if app.page == "tempo" {
		paintTempo(hdc, w, h)
	} else if app.page == "timeline" {
		paintTimeline(hdc, w, h)
	} else {
		textDraw(hdc, "YOUR LIBRARY", 283, 278, 280, 26, palette.white, app.itemFont, DT_LEFT)
		labelCount(hdc, len(app.rows), 408, 278)
		fill(hdc, 283, 350, w-315, 1, palette.line)
		// search bg
		rounded(hdc, 283, 310, w-585+60, 35, palette.edit, palette.border, 12)
		textDraw(hdc, "⌕", 295, 313, 20, 29, palette.muted, app.headingFont, DT_CENTER)
		textDraw(hdc, "IMPORT FILES TO ADD MACROS", w-303, 310, 262, 35, palette.subtle, app.microFont, DT_RIGHT)
		maxVisible := (h - 462) / 63
		if maxVisible < 1 {
			maxVisible = 1
		}
		for row := 0; row < maxVisible && row+app.scroll < len(app.rows); row++ {
			idx := app.rows[row+app.scroll]
			m := app.items[idx]
			x, y, rw, rh := selectedRowRect(row + app.scroll)
			chosen := strings.EqualFold(app.selected, m.Path)
			bg := palette.panel
			bd := palette.line
			if chosen {
				bg = 0x232A4A
				bd = 0x6256A1
			}
			if app.hovered == "row:"+m.Path && !chosen {
				bg = palette.panelHover
			}
			rounded(hdc, x, y, rw, rh, bg, bd, 14)
			rounded(hdc, x+11, y+10, 39, 35, 0x2B3455, 0x36466B, 10)
			sym := "AHK"
			if strings.EqualFold(filepath.Ext(m.Path), ".exe") {
				sym = "EXE"
			}
			textDraw(hdc, sym, x+14, y+12, 33, 29, palette.accent2, app.microFont, DT_CENTER)
			name := m.Name
			if name == "" {
				name = displayName(m.Path)
			}
			textDraw(hdc, name, x+63, y+5, rw-244, 28, palette.white, app.itemFont, DT_LEFT)
			textDraw(hdc, m.Path, x+63, y+31, rw-244, 19, palette.subtle, app.microFont, DT_LEFT)
			if isRunning(m.Path) {
				rounded(hdc, x+rw-162, y+15, 80, 25, 0x163D41, 0x286A6F, 11)
				textDraw(hdc, "● LIVE", x+rw-159, y+15, 74, 25, palette.green, app.microFont, DT_CENTER)
			}
			star := "☆"
			clr := palette.muted
			if m.Favorite {
				star = "★"
				clr = palette.accent2
			}
			textDraw(hdc, star, x+rw-57, y+9, 37, 38, clr, app.headingFont, DT_CENTER)
			hit("favorite:"+m.Path, x+rw-60, y+7, 47, 42)
			hit("row:"+m.Path, x+2, y+3, rw-66, rh-6)
		}
		if len(app.rows) == 0 {
			rounded(hdc, 283, 361, w-315, 173, palette.panel, palette.line, 16)
			textDraw(hdc, "No macros found", 307, 389, w-360, 36, palette.white, app.headingFont, DT_CENTER)
			txt := "Click IMPORT MACROS to add .ahk or .exe files."
			if app.page == "favorites" {
				txt = "Star a macro from your library to pin it here."
			}
			if app.page == "running" {
				txt = "Launch a macro from the Library to see it here."
			}
			if app.query != "" {
				txt = "Try a different search or clear the search field."
			}
			textDraw(hdc, txt, 307, 432, w-360, 30, palette.muted, app.subFont, DT_CENTER)
		}
		// Actions footer
		button(hdc, "rename", "RENAME", 284, h-96, 109, 37, 0)
		button(hdc, "folder", "OPEN FOLDER", 402, h-96, 136, 37, 0)
		button(hdc, "remove", "REMOVE", 547, h-96, 113, 37, 3)
		button(hdc, "tempo-open", "SPEED UP", 672, h-96, 131, 37, 2)
		button(hdc, "tl-open", "EDIT TIMELINE", 813, h-96, 177, 37, 1)
	}
	fill(hdc, 282, h-45, w-314, 1, palette.line)
	textDraw(hdc, byteshort(app.status, 115), 284, h-39, w-350, 29, palette.muted, app.subFont, DT_LEFT)
	pBitBlt.Call(realDC, 0, 0, uintptr(w), uintptr(h), hdc, 0, 0, 0x00CC0020)
	pSelectObject.Call(memDC, oldBitmap)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(memDC)
}
func favoriteCount() int {
	n := 0
	for _, m := range app.items {
		if m.Favorite {
			n++
		}
	}
	return n
}
func paintSettings(hdc uintptr, w, h int) {
	rounded(hdc, 283, 298, w-315, 253, palette.panel, palette.border, 18)
	textDraw(hdc, "AUTOHOTKEY V2", 311, 314, w-400, 27, palette.accent2, app.microFont, DT_LEFT)
	textDraw(hdc, "Interpreter path", 311, 343, w-400, 29, palette.white, app.headingFont, DT_LEFT)
	p := app.interpreter
	if p == "" {
		p = app.detectedAHK
	}
	if p == "" {
		p = "Not detected — choose your AutoHotkey v2 executable."
	}
	rounded(hdc, 312, 395, w-376, 39, palette.edit, palette.line, 11)
	textDraw(hdc, p, 326, 396, w-407, 37, palette.muted, app.subFont, DT_LEFT)
	button(hdc, "chooseahk", "CHOOSE EXE", 312, 454, 148, 40, 1)
	button(hdc, "resetahk", "AUTO DETECT", 470, 454, 148, 40, 0)
	textDraw(hdc, "Escape stays untouched. Mode selection is saved automatically.", 314, 505, w-400, 31, palette.green, app.subFont, DT_LEFT)
	rounded(hdc, 283, 569, w-315, 71, palette.panel, palette.line, 15)
	textDraw(hdc, "STORAGE", 307, 576, 180, 23, palette.accent2, app.microFont, DT_LEFT)
	textDraw(hdc, app.cfgPath, 307, 601, w-369, 24, palette.muted, app.subFont, DT_LEFT)
}

// AI Studio is drawn with native Win32 edit fields over a custom-designed dashboard.
// It deliberately never binds Escape or any global keyboard shortcuts.
func paintAI(hdc uintptr, w, h int) {
	right := w - 31
	colW := (right - 283 - 16) / 2
	x2 := 283 + colW + 16
	rounded(hdc, 283, 284, right-283, 67, palette.panel, palette.border, 15)
	textDraw(hdc, "API KEY", 304, 290, 122, 30, palette.accent2, app.microFont, DT_LEFT)
	statusText := "NOT CONNECTED"
	keyColor := palette.red
	if app.aiKeyPresent {
		statusText = "KEY SAVED SECURELY"
		keyColor = palette.green
	}
	textDraw(hdc, statusText, 303, 317, 123, 21, keyColor, app.microFont, DT_LEFT)
	rounded(hdc, 426, 302, x2-439, 36, palette.edit, palette.border, 10)
	button(hdc, "ai-connect", "SAVE KEY", x2+3, 302, 110, 36, 1)
	button(hdc, "ai-forget", "REMOVE KEY", x2+124, 302, 132, 36, 3)
	button(hdc, "ai-test", "TEST", x2+266, 302, 102, 36, 0)
	textDraw(hdc, "Private key storage: Windows DPAPI · no keys embedded in EXE", 284, 352, w-314, 23, palette.subtle, app.microFont, DT_LEFT)
	textDraw(hdc, "AI MODEL", 284, 376, 115, 24, palette.accent2, app.microFont, DT_LEFT)
	modelLabels := []struct{ ID, Title, Model string }{{"ai-model-luna", "LUNA  ·  LOW COST", "gpt-6-luna"}, {"ai-model-terra", "TERRA  ·  BALANCED", "gpt-6.1-terra"}, {"ai-model-sol", "SOL  ·  ADVANCED", "gpt-6.1-sol"}}
	for i, model := range modelLabels {
		st := 0
		if app.aiModel == model.Model {
			st = 1
		}
		button(hdc, model.ID, model.Title, 372+i*168, 374, 160, 33, st)
	}
	rounded(hdc, 283, 418, colW, h-519, palette.panel, palette.border, 16)
	rounded(hdc, x2, 418, colW, h-519, palette.panel, palette.border, 16)
	textDraw(hdc, "EDIT MODE", 300, 425, 94, 30, palette.accent2, app.microFont, DT_LEFT)
	editW := (colW - 120) / 2
	editPreserveStyle, editCreativeStyle := 0, 0
	if app.aiEditMode == aiEditCreative {
		editCreativeStyle = 1
	} else {
		editPreserveStyle = 1
	}
	button(hdc, "ai-edit-preserve", "PRESERVE NOTES", 390, 423, editW, 31, editPreserveStyle)
	button(hdc, "ai-edit-creative", "CREATIVE ARRANGE", 396+editW, 423, editW, 31, editCreativeStyle)
	textDraw(hdc, "AI RESPONSE / GENERATED CODE", x2+16, 425, colW-30, 30, palette.white, app.itemFont, DT_LEFT)
	rounded(hdc, 293, 459, colW-20, h-603, palette.edit, palette.line, 10)
	rounded(hdc, x2+10, 459, colW-20, h-603, palette.edit, palette.line, 10)
	btnY := h - 145
	button(hdc, "ai-chat", "ASK AI", 299, btnY, 101, 35, 0)
	button(hdc, "ai-generate", "GENERATE AHK", 407, btnY, 146, 35, 1)
	button(hdc, "ai-fix", "FIX SELECTED", 560, btnY, colW-279, 35, 2)
	textDraw(hdc, "Macro file name:", x2+16, h-146, 130, 26, palette.muted, app.microFont, DT_LEFT)
	button(hdc, "ai-save", "SAVE AS MACRO", x2+colW-154, h-145, 144, 35, 2)
	button(hdc, "ai-copy", "COPY RESPONSE", x2+colW-154, h-100, 144, 28, 0)
	button(hdc, "ai-preview", "PLAY AI PREVIEW", x2+16, h-98, 151, 28, 0)
	button(hdc, "ai-preview-stop", "STOP PREVIEW", x2+174, h-98, 145, 28, 0)
	info := "Creative Mode may use several API calls. It edits note data and rebuilds a complete AHK."
	if app.aiBusy {
		info = "AI is processing your request ... the launcher stays responsive."
	}
	textDraw(hdc, info, 287, h-103, w-470, 27, palette.muted, app.microFont, DT_LEFT)
}

func showFileDialog(interpreter bool) {
	buf := make([]uint16, 32768)
	filters := "AutoHotkey macros (*.ahk)\x00*.ahk\x00Executable macros (*.exe)\x00*.exe\x00All supported (*.ahk;*.exe)\x00*.ahk;*.exe\x00\x00"
	title := "Import AutoHotkey macro files"
	if interpreter {
		filters = "AutoHotkey v2 executable (*.exe)\x00*.exe\x00\x00"
		title = "Select AutoHotkey v2 executable"
	}
	f := utfMultiSZ(filters)
	ofn := openFilenameW{LStructSize: uint32(unsafe.Sizeof(openFilenameW{})), HwndOwner: app.hwnd,
		Filter: &f[0], File: &buf[0], MaxFile: uint32(len(buf)), Title: utf(title),
		Flags: OFN_EXPLORER | OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST}
	if !interpreter {
		ofn.Flags |= OFN_ALLOWMULTISELECT
	}
	ret, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		if code, _, _ := pCommDlgExtendedError.Call(); code != 0 {
			status(fmt.Sprintf("Import file picker failed (Windows error %d)", code))
		}
		return
	}
	parts := []string{}
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] == 0 {
			if i == start {
				break
			}
			parts = append(parts, syscall.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}
	if len(parts) == 0 {
		return
	}
	if interpreter {
		app.interpreter = parts[0]
		app.detectedAHK = findAHK()
		save()
		status("AutoHotkey interpreter saved.")
		return
	}
	paths := []string{}
	if len(parts) == 1 {
		paths = append(paths, parts[0])
	} else {
		for _, n := range parts[1:] {
			paths = append(paths, filepath.Join(parts[0], n))
		}
	}
	addPaths(paths)
}
func openFolder() {
	idx := selectedIndex()
	if idx < 0 {
		status("Select a macro first.")
		return
	}
	path := app.items[idx].Path
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); err != nil {
		status("That folder no longer exists.")
		return
	}
	verb := utf("open")
	target := utf(dir)
	pShellExecuteW.Call(app.hwnd, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, SW_SHOWNORMAL)
}
func startRename() {
	idx := selectedIndex()
	if idx < 0 {
		status("Select a macro first.")
		return
	}
	app.renamePath = app.items[idx].Path
	setText(app.rename, app.items[idx].Name)
	ypos := 315
	for i, ii := range app.rows {
		if strings.EqualFold(app.items[ii].Path, app.renamePath) && i >= app.scroll {
			y := 358 + (i-app.scroll)*63
			if y+33 < app.h-102 {
				ypos = y + 10
			}
			break
		}
	}
	pMoveWindow.Call(app.rename, 348, uintptr(ypos), uintptr(app.w-695), 33, 1)
	pShowWindow.Call(app.rename, SW_SHOW)
	pSetFocus.Call(app.rename)
	pSendMessageW.Call(app.rename, EM_SETSEL, 0, uintptr(^uint(0)))
	status("Type a display name, then click elsewhere to save.")
}
func finishRename() {
	if app.renamePath == "" {
		return
	}
	name := strings.TrimSpace(getText(app.rename))
	path := app.renamePath
	app.renamePath = ""
	pShowWindow.Call(app.rename, SW_HIDE)
	if name != "" {
		for i := range app.items {
			if strings.EqualFold(app.items[i].Path, path) {
				app.items[i].Name = name
				break
			}
		}
		save()
		status("Macro renamed in your library.")
	}
}

// Centralized page navigation. A converter completion, a manual import,
// and sidebar navigation must all synchronize Win32 child controls.
func switchPage(page string) {
	app.page = page
	app.scroll = 0
	updateEditorVisibility()
	redraw()
}
func click(id string) {
	if clickTimelineAction(id) { return }
	if clickUpdateAction(id) {
		return
	}
	switch {
	case strings.HasPrefix(id, "nav:"):
		finishRename()
		switchPage(strings.TrimPrefix(id, "nav:"))
	case strings.HasPrefix(id, "row:"):
		app.selected = strings.TrimPrefix(id, "row:")
		redraw()
	case strings.HasPrefix(id, "favorite:"):
		toggleFavorite(strings.TrimPrefix(id, "favorite:"))
	case id == "import":
		showFileDialog(false)
	case id == "run":
		runSelected()
	case id == "stop":
		stopSelected()
	case id == "stopall":
		stopAll(true)
	case id == "mode-single":
		setMode("single")
	case id == "mode-multi":
		setMode("multi")
	case id == "rename":
		startRename()
	case id == "remove":
		removeSelected()
	case id == "folder":
		openFolder()
	case id == "tempo-open":
		openTempoLab()
	case id == "tempo-back":
		switchPage("library")
	case id == "tempo-create":
		createTempoVariant()
	case strings.HasPrefix(id, "tempo-pick:"):
		value, e := strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(id, "tempo-pick:"), "x"), 64)
		if e == nil {
			pickTempoSpeed(value)
		}
	case id == "chooseahk":
		showFileDialog(true)
	case id == "instrument-install":
		openCaptureInstall()
	case id == "instrument-start":
		beginInstrumentCapture()
	case id == "instrument-cancel":
		cancelInstrumentCapture()
	case id == "instrument-open":
		openInstrumentFolder()
	case id == "lyre-import":
		chooseLyreFile()
	case id == "lyre-find":
		showSongSearch()
	case id == "song-search-go":
		startSongNameSearch()
	case id == "song-search-back":
		switchPage("lyre")
	case id == "song-search-import":
		switchPage("lyre")
		chooseLyreFile()
	case strings.HasPrefix(id, "song-source:"):
		n, e := strconv.Atoi(strings.TrimPrefix(id, "song-source:"))
		if e == nil {
			openSongSource(n)
		}
	case id == "lyre-convert":
		lyreConvertSelected()
	case id == "lyre-minus-10":
		nudgeLyreOffset(-10)
	case id == "lyre-minus-1":
		nudgeLyreOffset(-1)
	case id == "lyre-plus-1":
		nudgeLyreOffset(1)
	case id == "lyre-plus-10":
		nudgeLyreOffset(10)
	case id == "lyre-plus-60":
		nudgeLyreOffset(60)
	case id == "lyre-quality-clean":
		setQuality(qualityClean)
	case id == "lyre-quality-balanced":
		setQuality(qualityBalanced)
	case id == "lyre-quality-full":
		setQuality(qualityFull)
	case id == "lyre-arrange-standard":
		setLyreArrangeMode(lyreArrangeStandard)
	case id == "lyre-arrange-creative":
		setLyreArrangeMode(lyreArrangeCreative)
	case id == "lyre-arrange-agent":
		setLyreArrangeMode(lyreArrangeAgent)
	case id == "lyre-agent-budget":
		if !app.lyreBusy {
			app.agentBudgetCents = nextAgentBudget(app.agentBudgetCents)
			save()
			status(fmt.Sprintf("Agent Pro budget guard: $%.2f per song (estimated model tokens, not an account billing limit).", float64(app.agentBudgetCents)/100))
			redraw()
		}
	case id == "lyre-preview":
		beginLyrePreview()
	case id == "lyre-preview-sampled":
		beginSampledLyrePreview()
	case id == "lyre-preview-stop":
		stopLyrePreview()
		status("Lyre preview stopped.")
	case id == "lyre-export":
		exportLyreNow()
	case id == "lyre-setup":
		setupAudioEngine()
	case id == "lyre-setup-video":
		setupVideoEngine()
	case id == "video-visual":
		setVideoMode(videoVisual)
	case id == "video-overlay":
		setVideoMode(videoOverlay)
	case id == "video-hybrid":
		setVideoMode(videoHybrid)
	case id == "video-audio":
		setVideoMode(videoAudio)
	case id == "ai-connect":
		connectAIKey()
	case id == "ai-forget":
		forgetAIKey()
	case id == "ai-test":
		startAIRequest("test")
	case id == "ai-chat":
		startAIRequest("chat")
	case id == "ai-generate":
		startAIRequest("generate")
	case id == "ai-fix":
		startAIRequest("fix")
	case id == "ai-edit-preserve":
		setAIEditMode(aiEditPreserve)
	case id == "ai-edit-creative":
		setAIEditMode(aiEditCreative)
	case id == "ai-preview":
		beginCreativePreview()
	case id == "ai-preview-stop":
		stopLyrePreview()
		status("AI preview stopped.")
	case id == "ai-save":
		saveAIMacro()
	case id == "ai-copy":
		copyAIOutput()
	case id == "ai-model-luna":
		app.aiModel = "gpt-6-luna"
		save()
		status("Selected Luna: lowest-cost model.")
	case id == "ai-model-terra":
		app.aiModel = "gpt-6.1-terra"
		save()
		status("Selected Terra: balanced model.")
	case id == "ai-model-sol":
		app.aiModel = "gpt-6.1-sol"
		save()
		status("Selected Sol: stronger coding model, higher cost.")
	case id == "resetahk":
		app.interpreter = ""
		app.detectedAHK = findAHK()
		save()
		status("Auto-detecting AutoHotkey v2. Current: " + app.detectedAHK)
	}
}
func at(x, y int) string {
	for i := len(app.hits) - 1; i >= 0; i-- {
		z := app.hits[i]
		if x >= z.X && x < z.X+z.W && y >= z.Y && y < z.Y+z.H {
			return z.ID
		}
	}
	return ""
}
func winProc(hwnd uintptr, message uint32, wParam, lParam uintptr) (out uintptr) {
	// Capture unexpected UI callback failures for troubleshooting.
	defer func() {
		if x := recover(); x != nil {
			logPath := filepath.Join(filepath.Dir(appDataPath()), "last_error.log")
			_ = os.MkdirAll(filepath.Dir(logPath), 0755)
			log := fmt.Sprintf("Cazy Lyre v3.11.0 crash\nMessage: 0x%X\nError: %v\n%s\n", message, x, debug.Stack())
			_ = os.WriteFile(logPath, []byte(log), 0644)
			out = 0
		}
	}()
	switch message {
	case WM_CREATE:
		app.hwnd = hwnd
		app.page = "library"
		app.mode = "single"
		app.aiModel = "gpt-6-luna"
		app.videoMode = videoVisual
		app.lyreArrangeMode = lyreArrangeStandard
		app.lyreQuality = qualityFull
		app.tempoSpeed = 1.25
		app.sessions = make(map[string]*runningSession)
		loadUpdaterPrefs()
		app.status = "Welcome to Cazy Lyre. Open Lyre Converter to preview a MIDI or MP3 arrangement before export."
		load()
		app.brush = make(map[uint32]uintptr)
		app.pen = make(map[uint32]uintptr)
		app.itemFont = font(16, 580)
		app.headingFont = font(21, 650)
		app.titleFont = font(30, 700)
		app.subFont = font(14, 420)
		app.microFont = font(12, 640)
		app.brandFont = font(17, 760)
		app.editBrush = makeBrush(palette.edit)
		// Native text boxes preserve keyboard input and accessibility.
		editClass := utf("EDIT")
		app.search, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|WS_VISIBLE|ES_AUTOHSCROLL, uintptr(324), uintptr(315), uintptr(460), uintptr(26),
			hwnd, ID_EDIT_SEARCH, 0, 0)
		pSendMessageW.Call(app.search, WM_SETFONT, app.subFont, 1)
		placeholder := utf("Search by name or file path...")
		pSendMessageW.Call(app.search, EM_SETCUEBANNER, 1, uintptr(unsafe.Pointer(placeholder)))
		app.rename, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|WS_BORDER|ES_AUTOHSCROLL, uintptr(325), uintptr(298), uintptr(400), uintptr(33),
			hwnd, ID_EDIT_RENAME, 0, 0)
		pSendMessageW.Call(app.rename, WM_SETFONT, app.subFont, 1)
		pShowWindow.Call(app.rename, SW_HIDE)
		app.keyEdit, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|ES_PASSWORD|ES_AUTOHSCROLL, 434, 307, 305, 26,
			hwnd, ID_EDIT_API_KEY, 0, 0)
		pSendMessageW.Call(app.keyEdit, WM_SETFONT, app.subFont, 1)
		pSendMessageW.Call(app.keyEdit, EM_SETCUEBANNER, 0, uintptr(unsafe.Pointer(utf("Paste NEW API key here"))))
		app.aiPrompt, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_WANTRETURN,
			298, 466, 350, 145, hwnd, ID_EDIT_AI_PROMPT, 0, 0)
		pSendMessageW.Call(app.aiPrompt, WM_SETFONT, app.subFont, 1)
		app.aiOutput, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_READONLY,
			702, 466, 350, 145, hwnd, ID_EDIT_AI_OUTPUT, 0, 0)
		pSendMessageW.Call(app.aiOutput, WM_SETFONT, app.subFont, 1)
		app.aiFileName, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|ES_AUTOHSCROLL,
			832, 618, 115, 24, hwnd, ID_EDIT_AI_FILENAME, 0, 0)
		pSendMessageW.Call(app.aiFileName, WM_SETFONT, app.subFont, 1)
		app.lyreSong, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|ES_AUTOHSCROLL, 522, 484, 350, 30,
			hwnd, ID_EDIT_LYRE_SONG, 0, 0)
		pSendMessageW.Call(app.lyreSong, WM_SETFONT, app.subFont, 1)
		app.songQuery, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|ES_AUTOHSCROLL, 471, 318, 300, 29,
			hwnd, ID_EDIT_SONG_QUERY, 0, 0)
		pSendMessageW.Call(app.songQuery, WM_SETFONT, app.subFont, 1)
		pSendMessageW.Call(app.songQuery, EM_SETCUEBANNER, 1, uintptr(unsafe.Pointer(utf("Type song title + artist here..."))))
		app.lyreOffset, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|ES_AUTOHSCROLL, 504, 530, 108, 30,
			hwnd, ID_EDIT_LYRE_SKIP, 0, 0)
		pSendMessageW.Call(app.lyreOffset, WM_SETFONT, app.subFont, 1)
		setText(app.lyreOffset, "0:00")
		app.lyreSpeed, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|ES_AUTOHSCROLL, 783, 530, 100, 30,
			hwnd, ID_EDIT_LYRE_SPEED, 0, 0)
		pSendMessageW.Call(app.lyreSpeed, WM_SETFONT, app.subFont, 1)
		setText(app.lyreSpeed, "1.0")
		app.urlEdit, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(editClass)), 0,
			WS_CHILD|ES_AUTOHSCROLL, 315, 470, 600, 28, hwnd, ID_EDIT_UPDATE_URL, 0, 0)
		pSendMessageW.Call(app.urlEdit, WM_SETFONT, app.subFont, 1)
		updater.urlBox = app.urlEdit
		setText(app.urlEdit, updater.repository)
		pShowWindow.Call(app.urlEdit, SW_HIDE)
		setText(app.aiFileName, "Cazy Lyre Macro")
		setText(app.aiOutput, "Cazy Lyre AI Studio\r\n\r\n1. Paste your NEW OpenAI API key above and click SAVE KEY.\r\n2. Type a request on the left.\r\n3. Choose ASK AI or GENERATE AHK.\r\n4. Review output, then SAVE AS MACRO.\r\n\r\nTo fix an existing AHK, first select it in Library, then use FIX SELECTED here.\r\n\r\nMouse 4 / Mouse 5 are default macro controls. Escape is never mapped.")
		// All AI input fields start hidden until the user opens AI Studio.
		// Do not reposition the search edit here: app dimensions are not known
		// until the first WM_PAINT on this native Win32 window.
		pDragAcceptFiles.Call(hwnd, 1)
		pSetTimer.Call(hwnd, 1, 750, 0)
		dark := int32(1)
		pDwmSetWindowAttribute.Call(hwnd, 20, uintptr(unsafe.Pointer(&dark)), unsafe.Sizeof(dark))
		return 0
	case WM_PAINT:
		paint(hwnd)
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_CTLCOLORSTATIC:
		pSetTextColor.Call(wParam, uintptr(colorref(palette.white)))
		pSetBkColor.Call(wParam, uintptr(colorref(palette.edit)))
		return app.editBrush
	case WM_CTLCOLOREDIT:
		pSetTextColor.Call(wParam, uintptr(colorref(palette.white)))
		pSetBkColor.Call(wParam, uintptr(colorref(palette.edit)))
		pSetBkMode.Call(wParam, 2)
		return app.editBrush
	case WM_COMMAND:
		id := loword(wParam)
		event := hiword(wParam)
		if id == ID_EDIT_SEARCH && event == EN_CHANGE {
			app.query = getText(app.search)
			app.scroll = 0
			redraw()
		}
		if event == EN_CHANGE && (id == ID_EDIT_LYRE_SONG || id == ID_EDIT_LYRE_SKIP || id == ID_EDIT_LYRE_SPEED) && len(app.lyrePrepared.Events) > 0 {
			clearLyrePrepared()
			status("Song settings changed. Click PREPARE PREVIEW to regenerate before export.")
		}
		if id == ID_EDIT_RENAME && event == EN_KILLFOCUS {
			finishRename()
		}
		return 0
	case WM_MOUSEMOVE:
		x, y := signedword(lParam), int(int16((lParam>>16)&0xFFFF))
		if app.page == "timeline" && (wParam & 1) != 0 && x>=312 && x<=app.w-67 && y>=382 && y<=457 {
			timelineScrub(x)
			return 0
		}
		m := at(x, y)
		if m != app.hovered {
			app.hovered = m
			redraw()
		}
		return 0
	case WM_LBUTTONDOWN:
		x, y := signedword(lParam), int(int16((lParam>>16)&0xFFFF))
		if app.page == "timeline" && x>=312 && x<=app.w-67 && y>=382 && y<=457 {
			timelineScrub(x)
			return 0
		}
		id := at(x, y)
		if app.renamePath != "" && id != "rename" {
			finishRename()
		}
		click(id)
		return 0
	case WM_LBUTTONDBLCLK:
		x, y := signedword(lParam), int(int16((lParam>>16)&0xFFFF))
		id := at(x, y)
		if strings.HasPrefix(id, "row:") {
			app.selected = strings.TrimPrefix(id, "row:")
			runSelected()
		}
		return 0
	case WM_MOUSEWHEEL:
		delta := int(int16((wParam >> 16) & 0xffff))
		if delta > 0 {
			app.scroll--
		} else {
			app.scroll++
		}
		visible := (app.h - 462) / 63
		if visible < 1 {
			visible = 1
		}
		filterRows()
		maxScroll := len(app.rows) - visible
		if maxScroll < 0 {
			maxScroll = 0
		}
		if app.scroll < 0 {
			app.scroll = 0
		}
		if app.scroll > maxScroll {
			app.scroll = maxScroll
		}
		redraw()
		return 0
	case WM_DROPFILES:
		n, _, _ := pDragQueryFileW.Call(wParam, 0xFFFFFFFF, 0, 0)
		paths := []string{}
		for i := uintptr(0); i < n; i++ {
			ln, _, _ := pDragQueryFileW.Call(wParam, i, 0, 0)
			buf := make([]uint16, int(ln)+2)
			pDragQueryFileW.Call(wParam, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			paths = append(paths, syscall.UTF16ToString(buf))
		}
		pDragFinish.Call(wParam)
		addPaths(paths)
		return 0
	case WM_KEYDOWN:
		if wParam == VK_F2 {
			startRename()
			return 0
		}
		if wParam == VK_RETURN && app.renamePath != "" {
			finishRename()
			return 0
		}
		// No Escape handler, no keyboard hooks.
	case WM_TIMER: // Enables live health monitoring without injecting any hotkeys.
		if !updater.autoChecked {
			updater.autoChecked = true
			checkUpdateOnline(true) // once at startup, without blocking the GUI
		}
		return 0
	case WM_APP + 2:
		// Worker has completed: GUI state is updated on this locked OS thread.
		select {
		case result := <-aiResults:
			app.aiBusy = false
			if result.Err != nil {
				setText(app.aiOutput, "Request failed:\r\n"+result.Err.Error())
				status("AI request failed. Read the result panel for details.")
			} else {
				// Restore protected note data before offering a new macro.
				// This prevents AI response truncation from erasing a song.
				if result.Kind == "fix" {
					restored, restoreErr := restoreLyreNotes(result.Text, result.MusicBlocks, result.OriginalNoteCount)
					if restoreErr != nil {
						app.aiLastGenerated = ""
						app.aiLastKind = ""
						setText(app.aiOutput, "Fix rejected to protect your music:\r\n"+restoreErr.Error()+"\r\n\r\nNothing was saved. Try FIX SELECTED again.")
						status("AI fix rejected: song notes were not preserved. Nothing saved.")
						return 0
					}
					result.Text = restored
				}
				if result.Kind == "fix" || result.Kind == "generate" || result.Kind == "creative" {
					if validateErr := validateNonemptyAHK(result.Text); validateErr != nil {
						app.aiLastGenerated = ""
						app.aiLastKind = ""
						setText(app.aiOutput, "AI output rejected:\r\n"+validateErr.Error()+"\r\nNothing was saved.")
						status("AI output has no working playback; nothing saved.")
						return 0
					}
				}
				setText(app.aiOutput, result.Text)
				app.aiLastKind = result.Kind
				if result.Kind == "generate" || result.Kind == "fix" || result.Kind == "creative" {
					app.aiCreativeScore = result.CreativeScore
					app.aiLastGenerated = result.Text
					status("Macro ready. Review or PLAY AI PREVIEW before SAVE AS MACRO.")
				} else {
					app.aiLastGenerated = ""
					status("AI response ready. Your API key stays encrypted on this PC.")
				}
			}
		default:
		}
		return 0
	case WM_APP + 8:
		handleUpdateEvent()
		return 0
	case WM_APP + 7:
		handleCaptureUpdate()
		return 0
	case WM_APP + 6:
		select {
		case result := <-songSearchResults:
			app.songSearchBusy = false
			app.songSearchSources = result.Sources
			app.songSearchSummary = result.Summary
			if result.Err != nil {
				status("Song search: " + byteshort(result.Err.Error(), 140))
				msgBox("Cazy Lyre Song Search", result.Err.Error(), MB_OK|MB_ICONERROR)
			} else {
				status(fmt.Sprintf("Found %d credited sources. Click a result to open it, then import a downloaded song.", len(result.Sources)))
			}
		default:
		}
		return 0
	case WM_APP + 5:
		select {
		case update := <-creativeProgress:
			status(update)
		default:
		}
		return 0
	case WM_APP + 3:
		select {
		case result := <-lyreResults:
			app.lyreBusy = false
			if result.Err != nil {
				status("Lyre conversion failed: " + byteshort(result.Err.Error(), 130))
				msgBox("Cazy Lyre Conversion", result.Err.Error(), MB_OK|MB_ICONERROR)
			} else if result.Score.Quality != app.lyreQuality || result.Mode != app.lyreArrangeMode {
				status("Quality or arrangement mode changed. Click PREPARE PREVIEW again.")
			} else {
				// Preview is a non-destructive stage. Export to Library only
				// when the user explicitly approves the arranged notes.
				app.lyrePrepared = result.Score
				app.lyrePreparedTitle = result.Title
				app.lyrePreparedMode = result.Mode
				qualityNote := ""
				if result.Score.CreativeKeptSections > 0 || result.Score.CreativeChordRepairs > 0 {
					qualityNote = fmt.Sprintf(" · AI recovered %d chords; retained %d original sections", result.Score.CreativeChordRepairs, result.Score.CreativeKeptSections)
				}
				if result.Score.AgentCalls > 0 {
					qualityNote += fmt.Sprintf(" · Agent %d calls, estimated $%.3f", result.Score.AgentCalls, result.Score.AgentSpentUSD)
				}
				status(fmt.Sprintf("PREVIEW READY: %s from %s · %s · %d events · %.1f sec%s. PLAY, then EXPORT + ADD.",
					lyreArrangeDisplay(result.Mode), formatLyreStartTime(result.Score.SourceStartMs), qualityDisplay(result.Score.Quality), len(result.Score.Events),
					float64(result.Score.DurationMs)/1000, qualityNote))
			}
		default:
		}
		return 0
	case WM_APP + 4:
		select {
		case r := <-previewResults:
			if r.ID == app.lyrePreviewID {
				app.lyrePreviewCancel = nil
				if r.Err != nil {
					status("Preview audio unavailable: " + r.Err.Error())
				} else {
					status("Preview finished. Export when you're happy with the arrangement.")
				}
			}
		default:
		}
		return 0
	case WM_APP + 1:
		// Only remove a session if the completed PID still matches its tracked session.
		// A previously stopped process must not clear a newly launched replacement.
		for key, sess := range app.sessions {
			if sess.PID == int(wParam) {
				delete(app.sessions, key)
				app.status = fmt.Sprintf("%s finished. %d macro(s) remain active.", sess.Name, activeCount())
				redraw()
				break
			}
		}
		return 0
	case WM_CLOSE:
		if app.captureCancel != nil {
			app.captureCancel()
		}
		stopLyrePreview()
		stopAll(false)
		pDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		pKillTimer.Call(hwnd, 1)
		for _, v := range app.brush {
			pDeleteObject.Call(v)
		}
		for _, v := range app.pen {
			pDeleteObject.Call(v)
		}
		for _, f := range []uintptr{app.itemFont, app.headingFont, app.titleFont, app.subFont, app.microFont, app.brandFont} {
			pDeleteObject.Call(f)
		}
		pPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := pDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}
func createIcon(target int) uintptr {
	if len(iconData) < 6 {
		return 0
	}
	count := int(binary.LittleEndian.Uint16(iconData[4:6]))
	bestDiff := 99999
	var chosenOff, chosenLen int
	for i := 0; i < count; i++ {
		j := 6 + i*16
		if j+16 > len(iconData) {
			break
		}
		size := int(iconData[j])
		if size == 0 {
			size = 256
		}
		dataSize := int(binary.LittleEndian.Uint32(iconData[j+8 : j+12]))
		offset := int(binary.LittleEndian.Uint32(iconData[j+12 : j+16]))
		if offset < 0 || offset+dataSize > len(iconData) || dataSize == 0 {
			continue
		}
		diff := size - target
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			chosenOff = offset
			chosenLen = dataSize
		}
	}
	if chosenLen == 0 {
		return 0
	}
	h, _, _ := pCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&iconData[chosenOff])),
		uintptr(chosenLen), 1, 0x00030000, uintptr(target), uintptr(target), 0)
	return h
}
func font(size, weight int) uintptr {
	face := utf("Segoe UI")
	h, _, _ := pCreateFontW.Call(uintptr(int32(-size)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 0, 0, uintptr(unsafe.Pointer(face)))
	return h
}

// Re-launch with admin rights before creating the GUI.
// Windows then gives every child process the same elevated token.
// A normal UAC approval is required at each launch unless already elevated.
func requireAdmin() bool {
	elevated, _, _ := pIsUserAnAdmin.Call()
	if elevated != 0 {
		return true
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	verb := utf("runas")
	filename := utf(exe)
	cwd := utf(filepath.Dir(exe))
	result, _, _ := pShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(filename)), 0,
		uintptr(unsafe.Pointer(cwd)), SW_SHOWNORMAL)
	if result <= 32 {
		title := utf("Cazy Lyre — Administrator permission needed")
		body := utf("Cazy Lyre requests administrator access so launched macros also run as administrator. Approve the Windows UAC prompt if you want elevated macros.")
		pMessageBoxW.Call(0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), MB_OK|MB_ICONINFORMATION)
	}
	return false
}

func main() {
	// Win32 GUI message pumping and all window operations must stay on one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !requireAdmin() {
		return
	}
	pSetProcessDPIAware.Call()
	hinst, _, _ := pGetModuleHandleW.Call(0)
	cls := utf("CazyLyreGoWndClass")
	arrow, _, _ := pLoadCursorW.Call(0, 32512)
	bigIcon := createIcon(64)
	smallIcon := createIcon(32)
	wc := wndClassEx{
		Icon: bigIcon, IconSmall: smallIcon,
		CbSize:   uint32(unsafe.Sizeof(wndClassEx{})),
		Style:    CS_DBLCLKS,
		WndProc:  syscall.NewCallback(winProc),
		Instance: hinst, Cursor: arrow, ClassName: cls,
	}
	reg, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if reg == 0 {
		fmt.Println("Could not register window class:", err)
		return
	}
	title := utf("Cazy Lyre v3.11.0 | Windsong Lyre Studio")
	hwnd, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)),
		WS_OVERLAPPED|WS_CAPTION|WS_SYSMENU|WS_MINIMIZEBOX|WS_VISIBLE,
		CW_USEDEFAULT, CW_USEDEFAULT, 1120, 755, 0, 0, hinst, 0)
	if hwnd == 0 {
		fmt.Println("Could not create window:", err)
		return
	}
	pShowWindow.Call(hwnd, SW_SHOW)
	pUpdateWindow.Call(hwnd)
	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
