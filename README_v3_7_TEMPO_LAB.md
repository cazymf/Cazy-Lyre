# Cazy Lyre v3.7 — Tempo Lab

## New: Speed up AHK songs you already created

The Macro Library now includes **SPEED UP**. Pick an imported .ahk song, choose a playback-speed preset, and Cazy Lyre writes an entirely new script without changing the original.

### Available presets

- 1.00x — original speed (duplicate)
- 1.10x — slightly faster
- **1.25x** — 25% faster (a 100-second song takes about 80 seconds)
- **1.35x** — 35% faster (100 seconds becomes about 74 seconds)
- **1.50x** — 50% faster (100 seconds becomes about 67 seconds)
- 1.75x, 2.00x — faster options

Speeding up by 1.25x means the song lasts 1/1.25 as long, **not** 25% less timing precision. The notes, keys, chords, start/stop logic and existing controls remain otherwise unchanged. No OpenAI API call is made.

### How to use

1. Run `Cazy_Lyre_v3_7.exe` from the extracted folder.
2. In **Macro Library**, click an existing `.ahk` lyre song to select it. If you don't see it, click **IMPORT MACROS** and select its `.ahk` file.
3. Click **SPEED UP** in the library's bottom toolbar.
4. Choose **1.25x**, **1.35x**, **1.50x**, or another preset.
5. Click **CREATE FASTER COPY**. You'll return to the library with the newly created song selected.
6. Open Genshin's Windsong Lyre, focus the game, and use the selected script's existing **Launch** action. The app does not automatically launch a song just after export.

### Files and originals

Tempo variants are stored in `%LOCALAPPDATA%\MacroDeck Pro\Tempo Exports` (example: `Song_1_35x.ahk`). A second export with the same name gets a numeric suffix; neither export nor Cazy Lyre changes the original script. Macro Library's existing library.json remains in use, preserving your saved scripts and settings.

### Compatibility

Tempo Lab supports AutoHotkey v2 note-event arrays shaped like `[milliseconds, "lyrekeys"]`, with the array named `notes`, `Song`, `NoteEvents`, `events` or `SongEvents`. This includes the normal Cazy Lyre v3.x AHK outputs and several older timed scripts. Scripts with only raw `Sleep(...)` timing or custom event syntax are not blindly rewritten; the tool displays an error instead. It also refuses sources that already bind Escape; **Esc remains untouched**.

- Mouse 4, Mouse 5 and F12 bindings are preserved from the source (the tool doesn't add missing hotkeys).
- The original note pitches and order do not change; this is a tempo adjustment, **not an AI rearrangement**.
- If a very fast version sounds rushed or drops notes, try a lower preset. No real Windows/Genshin playback was available in the build environment; actual macro playback remains to be checked on your PC.

## Retained v3.6 features

- Lyre Capture: one-time, locally recorded 21-note Windsong Lyre instrument profile.
- REAL LYRE SOUND sampled preview.
- Agent Pro music director / arranger / critic, with spending guard.
- MIDI / MP3 / MP4 conversion, adaptive Genshin 21-key overlay detection.
- Single/Multi macro launcher modes, saved library, AI Studio and song search.
- Custom pink character app/desktop icon resources.

## What's next

The full automatic listen-to-rendered-song, compare-to-source, and repair-again loop remains planned work. This v3.7 release specifically implements **Tempo Lab** and does not claim that self-correction loop is already finished.
