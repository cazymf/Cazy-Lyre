# Cazy Lyre v3.10.0 — GitHub Song Sync

This version adds a **SYNC SONGS** button on the existing **Updates** screen.

## First-time use

1. Install the v3.10.0 Windows ZIP from GitHub Releases or use the existing app's **Install from ZIP** feature.
2. In Cazy Lyre, open **Updates**. Confirm repository is `cazymf/Cazy-Lyre`.
3. Click **SYNC SONGS**. New macros appear in **Macro Library**.
4. Launch with Mouse4; stop with Mouse5; exit with F12. Escape remains untouched.
5. In Macro Library select a synced song and choose **SPEED UP** in Tempo Lab to create a faster copy (your original stays unchanged).

## How the song feed works

- The app loads `song-library/index.json` from the `main` branch.
- It fetches each listed `song-library/songs/<id>.ahk` through GitHub's HTTPS raw-content server.
- It verifies the published SHA-256 checksum and validates the Cazy Lyre timed-note macro before saving.
- Downloads are stored in `%LOCALAPPDATA%\Cazy Lyre\Synced Songs` under SHA-256-versioned filenames.
- Repeat syncs skip unchanged files. Any edited copy is left untouched. Local macros, API settings, and song exports aren't cleared.
- Song syncing and app updating are independent; **new songs do not require publishing another Windows app release**.

## First synced song

**The One That Got Away (Katy Perry, Ver.2 Genshin cover)** — 841 timed macro events, 373 chord events, original cover tempo (1.00x). Transcribed by detecting the video overlay's cyan key flashes rather than inferring piano pitches. This is an automated transcription, and in-game listening still needs user confirmation.

## Publishing additional songs

Add a new `.ahk` to `song-library/songs/<id>.ahk` and a corresponding `{id,title,sha256}` record to `song-library/index.json`. The published checksum must match the literal UTF-8 bytes. Then users click **SYNC SONGS**.
