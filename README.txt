CAZY LYRE v3.0 - CREATIVE LYRE CONVERTER + LIVE SONG SEARCH
========================================================

NEW: Find Song by Name now really searches the web from inside Cazy Lyre.
1. Go to Lyre Converter > FIND SONG BY NAME.
2. Type a song title and optionally an artist into the text field.
3. Click SEARCH ONLINE. This uses your previously connected OpenAI API key
   and uses API credits for a live web search (GPT-6.1 Sol).
4. Click a cited source to open it in your browser. Download a playable
   MIDI file or a piano tutorial/video from the source.
5. Click IMPORT DOWNLOADED FILE (or BACK TO CONVERTER > IMPORT SONG).
6. Choose STANDARD or CREATIVE AI, PREPARE PREVIEW, then EXPORT + ADD.

Important: Search finds sources and provides cited links. It does NOT
magically convert a title into music or silently download copyrighted
files. You choose a source and import the file before note conversion.
Some songs have no accessible MIDI or piano tutorial, and web search can
return no usable source. Your search results are sourced, not invented.

NEW: Lyre Converter > Step 2 > STANDARD / CREATIVE AI.
Creative AI now runs during PREPARE PREVIEW and may rearrange imperfect
transcriptions before export. It is distinct from AI Studio > FIX SELECTED.
It uses your OpenAI API credits, original note events, and extracted MIDI
pitch references where available. It does not listen to original audio
and cannot guarantee perfect reconstruction of complicated songs.
Preview before export. Original files remain untouched.

Persistent features: Single/Multi macro mode, mouse 4 start, mouse 5 stop,
F12 macro exit, admin launcher, old library, API-key DPAPI storage,
MP3/MIDI/MP4 import, Full Arrangement default, time skip, audio preview.
Esc is never remapped, intercepted, or bound.

Build notes: This EXE is compiled for Windows 10/11 x64. The API web-search
workflow requires internet and OpenAI web-search tool access. Its actual
UI and web search could not be live-tested from this Linux build environment.

CAZY LYRE v3.0 - CREATIVE CONVERSION INSIDE LYRE CONVERTER
==========================================================

NEW: Lyre Converter > Step 2 > STANDARD / CREATIVE AI.

- STANDARD: original local visual/audio/MIDI converter, with Clean / Balanced / Full quality.
- CREATIVE AI: PREPARE first extracts note events, then the built-in music arranger sends bounded sections to the OpenAI Responses API using your locally encrypted key. The output is validated as lyre notes and turned into a full AHK; nothing is automatically saved or executed.
- Review with PLAY PREVIEW, then EXPORT + ADD TO LIBRARY. Select your mode again next time; the choice is saved.
- Full Arrangement remains the default quality. You can still select Clean / Balanced quality.
- AI creative conversion consumes API credits (potentially multiple requests for long songs); it does not send the audio/video itself to the model. The converter shares transcribed note events, and if available extracted MIDI pitches.
- Error/invalid AI output leaves your library unchanged. Shorten a lengthy song with Start at if >30 AI sections.
- Older AI Studio Preserve Notes / Creative Arrangement still work independently.
- Mouse 4: start; Mouse 5: stop; F12: exit. No Esc bindings.
- Windows 64-bit; import library from previous releases preserved.

CAZY LYRE v2.9 - CREATIVE ARRANGEMENT AI
=========================================
Windows 10/11 64-bit | Cazy_Lyre_v2_9.exe

NEW: TWO AI EDITING MODES
-------------------------
AI Studio has two selectable buttons above the request editor:

1. PRESERVE NOTES (safe, default): "Fix Selected" improves AutoHotkey
   playback/control code, but keeps every original musical event intact.

2. CREATIVE ARRANGE: "Fix Selected" uses the FULL built-in expert music-
   arrangement instructions. It can replace/transpose/add/remove notes,
   restructure chords, adjust octaves, and improve timing for the 21-key
   Genshin lyre. The model sees actual song event data, not just an AHK
   code dump. If original extracted MIDI reference data is available,
   it also receives original piano pitch/time data for each passage.
   Creative mode generates structured note arrays; the app rebuilds the
   AHK itself to avoid silent AI-generated script placeholders.

HOW TO USE CREATIVE MODE
------------------------
1. Close older Cazy Lyre and run v2.9. The prior macro library, encrypted
   API key, Favorites, Single/Multi Mode, and converter settings persist.
2. Select an original lyre song .ahk in Macro Library.
3. Open AI Studio, choose CREATIVE ARRANGE, and optionally type your
   desired changes into the request box (or leave it blank to use the
   built-in music-arranger instructions).
4. Click FIX SELECTED. The AI processes musical sections in order.
   Longer songs use several API calls and deduct API credits. Wait until
   the status says the result is ready; an unsuccessful section cancels
   export rather than saving an incomplete song.
5. Click PLAY AI PREVIEW to listen to the arranged notes in the Windows
   MIDI harp preview. Use STOP PREVIEW to stop playback.
6. If satisfied, enter an output filename and click SAVE AS MACRO. It
   saves a NEW AHK file and adds it to the Library. The original AHK
   remains untouched. Nothing is run automatically.

ORIGINAL MIDI REFERENCE - RECOMMENDED
-------------------------------------
When v2.9 converts an MP4 / MP3 / MIDI and you click EXPORT + ADD TO
LIBRARY, it also writes a local *.ahk.cazy-notes.json file containing
original extracted piano note-on events (pitch, onset), where available.
This is not an audio recording or an API credential. Creative Arrange
uses that data as a reference for the original melody even when the
original lyric video was mapped imperfectly to the Genshin C-major lyre.
Older macros imported from v2.8 have NO such sidecar, unless you
re-convert the source song using v2.9 and export a new AHK.

The OpenAI API receives the selected note events and optional MIDI note
reference, NOT the underlying MP3/MP4 audio bytes. As a result AI can
rearrange the extracted melody, but cannot recognize missing notes from
an unavailable original recording. Playback quality is not guaranteed.

VALIDATION + CONTROLS
---------------------
- Rejects missing/invalid JSON, unsupported keys, empty sections,
  wildly out-of-range timestamps, and excessive note deletion.
- Validated musical events are converted to complete AHK v2 code with
  a monotonic, timer-driven playback loop and preserved simultaneous
  chords. The user must explicitly save.
- Generated macro hotkeys: Mouse 4 = start/restart, Mouse 5 = stop,
  F12 = exit. Esc is NEVER bound, intercepted, or remapped.
- Preview sounds use the Windows MIDI harp, not Genshin audio.
- AI Creative Mode uses multiple paid API requests (one per ~70 note
  events) for longer songs. Select a cheaper model for initial tests.
- Selectable Single/Multi Macro Mode, admin launch, library, favorites,
  MP4 Visual/Hybrid/Audio modes, MP3/MIDI, time-skip arrows, and Full
  Arrangement default remain available.

BUILD AND TEST STATUS
---------------------
The Windows x64 EXE was cross-compiled and structural MIDI, note-array,
arrangement JSON, song preservation, and time-offset tests passed.
Windows GUI, live OpenAI API calls, Windows MIDI preview, and Genshin
playback could not be run-tested in this environment. First run is a
real-world integration test, not a verified end-to-end music conversion.

Python 3.11 and the previously installed Basic Pitch and OpenCV venvs
are used unchanged. The AI key remains Windows DPAPI-encrypted locally;
no API key is included in this package.

FILES
-----
- Cazy_Lyre_v2_9.exe - Windows GUI application
- *.go - source code, including built-in arranger prompt
- video_notes.py - Visual MP4 transcription engine
- build_windows.cmd - local Go build helper
- INSTALL_VIDEO_ENGINE.cmd - optional video environment setup
- Lyre_Demo.mid - tiny sample for conversion testing
