# Cazy Lyre v3.11.0 — Timeline Lab

The app now has a **visual timeline editor for `.ahk` songs**. No API key or AI credit is required.

## How to use

1. In **Macro Library**, select a song and click **EDIT TIMELINE**.
2. Click or drag on the cyan note-density timeline to move the white playback cursor.
3. Use **-10 s / -1 s / -0.1 s / +0.1 s / +1 s / +10 s** for precise placement.
4. Press **SET START** to skip the beginning, or **SET END** to stop before the original ending.
5. To remove an unwanted middle section, place the cursor at its beginning and choose **MARK CUT IN**; move to its end and choose **MARK CUT OUT**. Then select **ADD CUT**. You can repeat this for multiple sections, and use **UNDO LAST CUT** or **CLEAR CUTS**.
6. Click **SAVE EDITED COPY**. The new song is saved under `%LOCALAPPDATA%\MacroDeck Pro\Timeline Exports` and automatically selected in Macro Library. The original AHK remains unchanged.
7. Launch the edited song or open it in **Tempo Lab** to change playback speed.

**About the visual:** The cyan bars show **note-event density**, not an audio waveform, because AHK macros contain timestamps and keys rather than audio samples. Dark overlays show sections outside START/END, and mauve overlays show removed middle cuts. Green/red lines indicate START/END and white indicates the playhead.

This editor supports Cazy Lyre timed AHK v2 arrays (`notes := [...]`, `global notes := [...]`, `Song := [...]`, etc.) and preserves all non-note script code, chords, Mouse4, Mouse5, and F12. It never binds Escape. Unsupported scripts show an error without changing the original.

**Timing semantics:** An excluded range removes notes at its start but keeps notes at its end. Later notes are moved earlier by the duration of each removed range. Existing note groups remain in sync.
