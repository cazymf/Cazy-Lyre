//go:build windows

package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

//go:embed Update-Apply.ps1
var updateRunnerPS []byte

const ID_EDIT_UPDATE_URL = 1050

type updateEvent struct {
	Kind     string
	Manifest releaseManifest
	StageDir string
	Version  string
	Detail   string
	Err      error
	Silent   bool
}

var updateEvents = make(chan updateEvent, 4)
var updater struct {
	urlBox       uintptr
	repository   string
	autoChecked  bool
	busy         bool
	state        string
	latest       releaseManifest
	readyDir     string
	readyVersion string
}

type updaterPrefs struct {
	Repository string `json:"repository"`
}

func updatesDataDir() string {
	home := os.Getenv("LOCALAPPDATA")
	if home == "" {
		home = os.TempDir()
	}
	return filepath.Join(home, "Cazy Lyre", "updates")
}
func updaterPrefsPath() string { return filepath.Join(updatesDataDir(), "preferences.json") }

const defaultLyreRepo = "cazymf/Cazy-Lyre"

func loadUpdaterPrefs() {
	updater.repository = defaultLyreRepo
	data, e := os.ReadFile(updaterPrefsPath())
	if e == nil {
		var p updaterPrefs
		if json.Unmarshal(data, &p) == nil && validGithubRepo(p.Repository) == nil {
			updater.repository = strings.TrimSpace(p.Repository)
		}
	}
	updater.state = "Connected to GitHub Releases: " + updater.repository
}
func saveUpdaterRepo() error {
	s := strings.TrimSpace(getText(updater.urlBox))
	if e := validGithubRepo(s); e != nil {
		return e
	}
	data, _ := json.MarshalIndent(updaterPrefs{Repository: s}, "", "  ")
	if e := os.MkdirAll(updatesDataDir(), 0700); e != nil {
		return e
	}
	if e := os.WriteFile(updaterPrefsPath(), data, 0600); e != nil {
		return e
	}
	updater.repository = s
	return nil
}
func requireHTTPS(raw string) error {
	return requireGithubDownloadURL(raw)
}
func updaterHTTPClient() *http.Client {
	return &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(r *http.Request, v []*http.Request) error {
		if len(v) > 5 {
			return errors.New("too many redirects")
		}
		return requireHTTPS(r.URL.String())
	}}
}
func postUpdateEvent(e updateEvent) { updateEvents <- e; pPostMessageW.Call(app.hwnd, WM_APP+8, 0, 0) }
func checkUpdateOnline(silent bool) {
	if updater.busy {
		return
	}
	if e := saveUpdaterRepo(); e != nil {
		if !silent {
			msgBox("GitHub Repository", e.Error(), MB_OK|MB_ICONERROR)
		}
		return
	}
	repo := updater.repository
	updater.busy = true
	updater.state = "Checking GitHub Releases for " + repo + "..."
	redraw()
	go func() {
		m, e := fetchLatestGithubLyre(updaterHTTPClient(), repo)
		if e != nil {
			postUpdateEvent(updateEvent{Kind: "check", Err: e, Silent: silent})
			return
		}
		if m.Version == "" {
			postUpdateEvent(updateEvent{Kind: "check", Silent: silent, Detail: "No Cazy Lyre releases published yet on GitHub."})
			return
		}
		cmp, e := compareLyreVersions(m.Version, lyreVersion)
		if e != nil {
			postUpdateEvent(updateEvent{Kind: "check", Err: e, Silent: silent})
			return
		}
		if cmp <= 0 {
			postUpdateEvent(updateEvent{Kind: "check", Manifest: m, Silent: silent, Detail: "You're running the latest GitHub version (v" + lyreVersion + ")."})
			return
		}
		postUpdateEvent(updateEvent{Kind: "check", Manifest: m, Silent: silent, Detail: "GitHub update v" + m.Version + " available - DOWNLOAD + PREPARE to install."})
	}()
}
func beginDownloadUpdate() {
	if updater.busy || updater.latest.Version == "" {
		return
	}
	cmp, e := compareLyreVersions(updater.latest.Version, lyreVersion)
	if e != nil || cmp <= 0 {
		updater.state = "No newer online release currently available."
		redraw()
		return
	}
	updater.busy = true
	updater.state = "Downloading and verifying release (SHA-256)..."
	redraw()
	m := updater.latest
	go func() {
		base := updatesDataDir()
		if e := os.MkdirAll(base, 0700); e != nil {
			postUpdateEvent(updateEvent{Kind: "stage", Err: e})
			return
		}
		f, e := os.CreateTemp(base, "download-*.zip")
		if e != nil {
			postUpdateEvent(updateEvent{Kind: "stage", Err: e})
			return
		}
		zipPath := f.Name()
		defer os.Remove(zipPath)
		req, e := http.NewRequest("GET", m.URL, nil)
		if e != nil {
			f.Close()
			postUpdateEvent(updateEvent{Kind: "stage", Err: e})
			return
		}
		res, e := updaterHTTPClient().Do(req)
		if e != nil {
			f.Close()
			postUpdateEvent(updateEvent{Kind: "stage", Err: e})
			return
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			f.Close()
			postUpdateEvent(updateEvent{Kind: "stage", Err: fmt.Errorf("update download HTTP %d", res.StatusCode)})
			return
		}
		copied, e := io.Copy(f, io.LimitReader(res.Body, 450*1024*1024+1))
		res.Body.Close()
		closeErr := f.Close()
		if e == nil {
			e = closeErr
		}
		if e == nil && copied > 450*1024*1024 {
			e = errors.New("download too large")
		}
		if e == nil {
			e = verifyFileSHA256(zipPath, m.SHA256)
		}
		if e != nil {
			postUpdateEvent(updateEvent{Kind: "stage", Err: e})
			return
		}
		stage, e := os.MkdirTemp(base, "stage-")
		if e != nil {
			postUpdateEvent(updateEvent{Kind: "stage", Err: e})
			return
		}
		d, n, e := stageReleaseZip(zipPath, stage, m.Version)
		if e != nil {
			os.RemoveAll(stage)
			postUpdateEvent(updateEvent{Kind: "stage", Err: e})
			return
		}
		postUpdateEvent(updateEvent{Kind: "stage", StageDir: stage, Version: d.Version, Detail: fmt.Sprintf("Verified v%s (%d files). Click INSTALL + RESTART.", d.Version, n)})
	}()
}
func selectUpdateZip() {
	if updater.busy {
		return
	}
	buf := make([]uint16, 32768)
	flt := utfMultiSZ("Cazy Lyre release ZIP (*.zip)\x00*.zip\x00\x00")
	f := openFilenameW{LStructSize: uint32(unsafe.Sizeof(openFilenameW{})), HwndOwner: app.hwnd, Filter: &flt[0], File: &buf[0], MaxFile: uint32(len(buf)), Title: utf("Choose a Cazy Lyre update release ZIP"), Flags: OFN_EXPLORER | OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST}
	ret, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&f)))
	if ret == 0 {
		return
	}
	file := syscall.UTF16ToString(buf)
	updater.busy = true
	updater.state = "Inspecting and staging local ZIP..."
	redraw()
	go func() {
		base := updatesDataDir()
		if e := os.MkdirAll(base, 0700); e != nil {
			postUpdateEvent(updateEvent{Kind: "stage", Err: e})
			return
		}
		stage, e := os.MkdirTemp(base, "stage-")
		if e != nil {
			postUpdateEvent(updateEvent{Kind: "stage", Err: e})
			return
		}
		d, n, e := stageReleaseZip(file, stage, "")
		if e != nil {
			os.RemoveAll(stage)
			postUpdateEvent(updateEvent{Kind: "stage", Err: e})
			return
		}
		postUpdateEvent(updateEvent{Kind: "stage", StageDir: stage, Version: d.Version, Detail: fmt.Sprintf("Local update v%s ready (%d files). Click INSTALL + RESTART.", d.Version, n)})
	}()
}
func handleUpdateEvent() {
	select {
	case e := <-updateEvents:
		updater.busy = false
		if e.Err != nil {
			updater.state = "Update failed: " + e.Err.Error()
			status(updater.state)
			if !e.Silent {
				msgBox("Cazy Lyre Updater", updater.state, MB_OK|MB_ICONERROR)
			}
			return
		}
		switch e.Kind {
		case "check":
			updater.latest = e.Manifest
			updater.state = e.Detail
		case "stage":
			if updater.readyDir != "" {
				os.RemoveAll(updater.readyDir)
			}
			updater.readyDir = e.StageDir
			updater.readyVersion = e.Version
			updater.state = e.Detail
		}
		status(updater.state)
	default:
	}
}
func applyStagedUpdate() {
	if updater.busy || updater.readyDir == "" {
		return
	}
	m := fmt.Sprintf("Install Cazy Lyre v%s and restart the app?\n\nExisting macro library, API credentials and captured instruments are kept.\nA backup of any overwritten program files will be saved.", updater.readyVersion)
	title := utf("Cazy Lyre Updater")
	body := utf(m)
	yes, _, _ := pMessageBoxW.Call(app.hwnd, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), 0x04|MB_ICONINFORMATION)
	if yes != 6 {
		return
	}
	exe, e := os.Executable()
	if e != nil {
		msgBox("Cazy Lyre Updater", e.Error(), MB_OK|MB_ICONERROR)
		return
	}
	folder := filepath.Dir(exe)
	runner, e := os.CreateTemp(updatesDataDir(), "updater-*.ps1")
	if e != nil {
		msgBox("Cazy Lyre Updater", e.Error(), MB_OK|MB_ICONERROR)
		return
	}
	runnerPath := runner.Name()
	if _, e = runner.Write(updateRunnerPS); e != nil {
		runner.Close()
		msgBox("Cazy Lyre Updater", e.Error(), MB_OK|MB_ICONERROR)
		return
	}
	if e = runner.Close(); e != nil {
		msgBox("Cazy Lyre Updater", e.Error(), MB_OK|MB_ICONERROR)
		return
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", runnerPath, "-InstallDir", folder, "-StageDir", updater.readyDir, "-ParentPid", fmt.Sprint(os.Getpid()))
	cmd.Dir = folder
	if e = cmd.Start(); e != nil {
		msgBox("Cazy Lyre Updater", "Could not start installer: "+e.Error(), MB_OK|MB_ICONERROR)
		return
	}
	_ = cmd.Process.Release()
	updater.state = "Updater started. Closing Cazy Lyre so program files can be replaced..."
	pPostMessageW.Call(app.hwnd, WM_CLOSE, 0, 0)
}
func paintUpdates(hdc uintptr, w, h int) {
	rounded(hdc, 283, 285, w-315, 112, palette.panel, palette.border, 16)
	textDraw(hdc, "CURRENT VERSION", 308, 293, 200, 22, palette.accent2, app.microFont, DT_LEFT)
	textDraw(hdc, "Cazy Lyre v"+lyreVersion, 307, 318, w-385, 37, palette.white, app.headingFont, DT_LEFT)
	textDraw(hdc, "GitHub Releases: check, download, verify, install and restart", 309, 360, w-365, 24, palette.muted, app.subFont, DT_LEFT)
	rounded(hdc, 283, 409, w-315, 159, palette.panel, palette.border, 16)
	textDraw(hdc, "GITHUB UPDATES", 308, 417, 260, 21, palette.accent2, app.microFont, DT_LEFT)
	textDraw(hdc, "GitHub repository (owner/name), e.g. cazymf/Cazy-Lyre", 309, 440, w-365, 24, palette.white, app.subFont, DT_LEFT)
	rounded(hdc, 301, 468, w-347, 32, palette.edit, palette.line, 8)
	button(hdc, "up-save", "SAVE REPO", 307, 514, 129, 34, 0)
	button(hdc, "up-check", "CHECK FOR UPDATES", 446, 514, 182, 34, 1)
	if updater.latest.Version != "" {
		button(hdc, "up-download", "DOWNLOAD + PREPARE", 639, 514, 193, 34, 2)
	}
	button(hdc, "up-local", "INSTALL FROM ZIP", 305, 582, 196, 38, 1)
	if updater.readyDir != "" {
		button(hdc, "up-apply", "INSTALL + RESTART", 512, 582, 212, 38, 2)
	}
	msg := updater.state
	if updater.busy {
		msg = "WORKING: " + msg
	}
	textDraw(hdc, byteshort(msg, 135), 307, h-88, w-362, 33, palette.green, app.subFont, DT_LEFT)
}
func clickUpdateAction(id string) bool {
	switch id {
	case "up-save":
		if e := saveUpdaterRepo(); e != nil {
			msgBox("Update feed", e.Error(), MB_OK|MB_ICONERROR)
		} else {
			updater.state = "GitHub repository saved: " + updater.repository
			redraw()
		}
	case "up-check":
		checkUpdateOnline(false)
	case "up-download":
		beginDownloadUpdate()
	case "up-local":
		selectUpdateZip()
	case "up-apply":
		applyStagedUpdate()
	default:
		return false
	}
	return true
}
