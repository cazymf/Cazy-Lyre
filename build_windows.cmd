@echo off
cd /d "%~dp0"
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
if exist Cazy_Lyre.exe del /q Cazy_Lyre.exe
go build -trimpath -ldflags "-H=windowsgui -s -w" -o Cazy_Lyre.exe main.go arrange_mode.go ai.go creative_core.go creative_win.go midi_core.go lyre_win.go video_win.go preview_win.go ahk_music_guard.go start_time.go song_search_core.go song_search_win.go agent_pro_core.go agent_pro_win.go instrument_win.go sampled_preview_win.go tempo_core.go tempo_win.go update_core.go github_release.go update_win.go
if errorlevel 1 exit /b 1
where objcopy >nul 2>&1
if errorlevel 1 goto :without_icon
where py >nul 2>&1
if errorlevel 1 goto :without_icon
py -3 embed_icon_pe.py Cazy_Lyre.exe Cazy_Pink_Icon.ico
if errorlevel 1 echo Icon resource step failed. The EXE is still built.
goto :finish
:without_icon
echo Python and GNU objcopy are needed to embed the EXE icon when building from source.
echo You can use Create_Cazy_Lyre_Desktop_Shortcut.ps1 to set the icon on a shortcut instead.
:finish
echo Built Cazy_Lyre.exe
