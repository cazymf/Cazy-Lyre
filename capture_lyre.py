"""Cazy Lyre v3.6 — supervised, local Windsong Lyre sampler.

Captures Windows default speaker loopback, not a remote video/screen stream.
Only runs key presses after foreground Genshin validation. Never sends Escape.
Python 3.11 + soundcard + numpy, installed by INSTALL_CAPTURE_ENGINE.cmd.
"""
from __future__ import annotations

import argparse
import ctypes
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
import wave
from datetime import datetime, timezone
from pathlib import Path

KEYS = "QWERTYUASDFGHJZXCVBNM"
RATE = 48000
DURATION = 2.65
COUNTDOWN = 7


def progress(kind: str, **data):
    print(json.dumps({"type": kind, **data}, ensure_ascii=False), flush=True)


def window_process_name(pid: int) -> str:
    kernel32 = ctypes.windll.kernel32
    kernel32.OpenProcess.argtypes = (ctypes.c_ulong, ctypes.c_int, ctypes.c_ulong)
    kernel32.OpenProcess.restype = ctypes.c_void_p
    kernel32.QueryFullProcessImageNameW.argtypes = (ctypes.c_void_p, ctypes.c_ulong, ctypes.c_wchar_p, ctypes.POINTER(ctypes.c_ulong))
    kernel32.QueryFullProcessImageNameW.restype = ctypes.c_int
    kernel32.CloseHandle.argtypes = (ctypes.c_void_p,)
    handle = kernel32.OpenProcess(0x1000, False, pid)  # PROCESS_QUERY_LIMITED_INFORMATION
    if not handle:
        return ""
    try:
        length = ctypes.c_ulong(4096)
        buffer = ctypes.create_unicode_buffer(length.value)
        ok = kernel32.QueryFullProcessImageNameW(handle, 0, buffer, ctypes.byref(length))
        if ok:
            return os.path.basename(buffer.value).lower()
        return ""
    finally:
        kernel32.CloseHandle(handle)


def check_genshin_foreground():
    user32 = ctypes.windll.user32
    user32.GetForegroundWindow.restype = ctypes.c_void_p
    user32.GetWindowTextW.argtypes = (ctypes.c_void_p, ctypes.c_wchar_p, ctypes.c_int)
    user32.GetWindowThreadProcessId.argtypes = (ctypes.c_void_p, ctypes.POINTER(ctypes.c_ulong))
    hwnd = user32.GetForegroundWindow()
    if not hwnd:
        raise RuntimeError("No foreground window. Switch to Genshin and retry.")
    buf = ctypes.create_unicode_buffer(1024)
    user32.GetWindowTextW(hwnd, buf, len(buf))
    pid = ctypes.c_ulong(0)
    user32.GetWindowThreadProcessId(hwnd, ctypes.byref(pid))
    exe = window_process_name(int(pid.value))
    title = buf.value.lower()
    allowed = ("genshin", "yuanshen", "原神", "원신")
    if not any(part in title or part in exe for part in allowed):
        raise RuntimeError(
            "Genshin was not focused when capture began. "
            f"Foreground window: {buf.value or '(no title)'} / {exe or '(unknown process)'}. "
            "Open Windsong Lyre and switch back to Genshin during the countdown."
        )
    return buf.value, exe


def make_ahk_key_script(path: Path):
    # Use the same SendMode Event used by Cazy Lyre's working music macros.
    # No global hotkeys, no keyboard hooks, and absolutely no Esc mapping.
    path.write_text(
        '#Requires AutoHotkey v2.0\n'
        '#SingleInstance Off\n'
        'SendMode("Event")\n'
        'SetKeyDelay(25, 25)\n'
        'if A_Args.Length < 1\n'
        '    ExitApp()\n'
        'k := A_Args[1]\n'
        'if !InStr("QWERTYUASDFGHJZXCVBNM", StrUpper(k))\n'
        '    ExitApp()\n'
        'Send("{" k "}")\n', encoding="utf-8"
    )


def write_pcm(path: Path, audio, np):
    a = np.asarray(audio, dtype=np.float32)
    if a.ndim == 1:
        a = a[:, None]
    if a.shape[1] not in (1, 2):
        a = a[:, :2] if a.shape[1] > 2 else a.mean(axis=1, keepdims=True)
    pcm = (np.clip(a, -1, 1) * 32767).astype("<i2")
    with wave.open(str(path), "wb") as wf:
        wf.setnchannels(pcm.shape[1])
        wf.setsampwidth(2)
        wf.setframerate(RATE)
        wf.writeframes(pcm.tobytes())


def analyze_audio(audio, quiet, np):
    a = np.asarray(audio, dtype=np.float32)
    b = np.asarray(quiet, dtype=np.float32)
    peak = float(np.max(np.abs(a))) if a.size else 0.0
    rms = float(np.sqrt(np.mean(a * a))) if a.size else 0.0
    base = float(np.sqrt(np.mean(b * b))) if b.size else 0.0
    # A note needs to be both clearly audible and above pre-key background.
    valid = peak >= max(0.004, float(np.max(np.abs(b))) * 1.35) and rms >= max(0.0009, base * 1.25)
    if not valid:
        return valid, peak, rms, base, 0
    mono = np.max(np.abs(a), axis=1) if a.ndim == 2 else np.abs(a)
    window = int(RATE * 0.012)
    chunks = len(mono) // window
    levels = np.max(mono[:chunks * window].reshape(chunks, window), axis=1)
    indices = np.flatnonzero(levels > max(0.002, base * 2.0))
    onset = max(0, int(indices[0]) * window - int(RATE * 0.035)) if len(indices) else 0
    return valid, peak, rms, base, onset


def save_profile(dest: Path, scratch: Path):
    """Only replace a previously successful profile after ALL 21 notes are captured."""
    backup = dest.with_name(dest.name + "_previous")
    if backup.exists():
        shutil.rmtree(backup)
    had_existing = dest.exists()
    if had_existing:
        dest.rename(backup)
    try:
        scratch.rename(dest)
    except Exception:
        if had_existing and backup.exists():
            backup.rename(dest)
        raise
    if backup.exists():
        shutil.rmtree(backup)


def real_capture(dest: Path, ahk_path: str, countdown: int):
    try:
        import numpy as np
        import soundcard as sc
    except ImportError as e:
        raise RuntimeError("Capture engine missing. Click INSTALL CAPTURE ENGINE in Cazy Lyre first (numpy + soundcard).") from e

    if not Path(ahk_path).is_file():
        raise RuntimeError("AutoHotkey v2 interpreter not found. Install or select it in Cazy Lyre Settings.")
    speaker = sc.default_speaker()
    if speaker is None:
        raise RuntimeError("No default Windows speaker/output device found.")
    try:
        mic = sc.get_microphone(id=speaker.name, include_loopback=True)
    except Exception as e:
        raise RuntimeError("Windows speaker loopback unavailable. Choose a normal Windows default output device.") from e

    progress("ready", message=f"Loopback ready on {speaker.name}; switch to Genshin. Music Volume 0, SFX on.")
    for seconds in range(countdown, 0, -1):
        progress("countdown", seconds=seconds, message=f"Switch to Genshin Windsong Lyre: {seconds} seconds")
        time.sleep(1)
    title, exe = check_genshin_foreground()
    progress("ready", message=f"Recording Genshin window: {title or exe}. Don't switch windows until completed.")
    dest.parent.mkdir(parents=True, exist_ok=True)
    scratch = Path(tempfile.mkdtemp(prefix="cazy_samples_", dir=str(dest.parent)))
    try:
        ahk_script = scratch / "single_key_capture.ahk"
        make_ahk_key_script(ahk_script)
        results = []
        with mic.recorder(samplerate=RATE, blocksize=2048) as recorder:
            for index, key in enumerate(KEYS, 1):
                # Keep every injection scoped to Genshin. No global hooks.
                check_genshin_foreground()
                best = None
                for attempt in range(1, 3):
                    quiet = recorder.record(numframes=int(RATE * 0.30))
                    try:
                        task = subprocess.Popen([ahk_path, str(ahk_script), key.lower()],
                                                creationflags=0x08000000)  # CREATE_NO_WINDOW
                    except OSError as e:
                        raise RuntimeError(f"AHK could not send key {key}: {e}") from e
                    audio = recorder.record(numframes=int(RATE * DURATION))
                    try:
                        task.wait(timeout=2)
                    except subprocess.TimeoutExpired:
                        task.kill()
                        task.wait(timeout=2)
                    if task.returncode != 0:
                        raise RuntimeError(f"AutoHotkey failed sending {key} (exit {task.returncode}).")
                    ok, peak, rms, base, onset = analyze_audio(audio, quiet, np)
                    best = (ok, audio, peak, rms, base, onset, attempt)
                    if ok:
                        break
                    progress("retry", index=index, key=key, message=f"Weak note {key}; retrying capture...", attempt=attempt)
                ok, audio, peak, rms, base, onset, attempt = best
                if not ok:
                    raise RuntimeError(f"Could not hear note {key} above background (peak {peak:.4f}, baseline rms {base:.4f}). Check Windows output and game SFX; existing samples preserved.")
                out_path = scratch / f"{key}.wav"
                # Trimming onset prevents auto-playing a per-key AHK process from
                # adding accidental latency into the virtual instrument sample.
                cropped = audio[onset:] if onset else audio
                write_pcm(out_path, cropped, np)
                results.append({"key": key, "file": f"{key}.wav", "peak": round(peak, 5),
                                "rms": round(rms, 5), "baseline_rms": round(base, 5),
                                "onset_trim_ms": round(1000 * onset / RATE), "attempts": attempt})
                progress("note", index=index, total=21, key=key, message=f"Captured {key} ({index}/21)")
        manifest = {"format": "Cazy Lyre Instrument Profile v1", "instrument": "Genshin Windsong Lyre",
                    "created_utc": datetime.now(timezone.utc).isoformat(), "sample_rate": RATE,
                    "source": "Windows default speaker loopback with Genshin in foreground",
                    "keys": KEYS, "samples": results}
        (scratch / "instrument.json").write_text(json.dumps(manifest, indent=2), encoding="utf-8")
        try:
            ahk_script.unlink()
        except OSError:
            pass
        save_profile(dest, scratch)
        progress("complete", index=21, total=21, folder=str(dest), message="21 notes captured and stored. Ready for sampled lyre preview.")
    finally:
        if scratch.exists():
            shutil.rmtree(scratch, ignore_errors=True)


def self_test(dest: Path):
    """Offline integration test: sample filesystem, WAV files, manifest and replacement."""
    import numpy as np
    dest.parent.mkdir(parents=True, exist_ok=True)
    scratch = Path(tempfile.mkdtemp(prefix="cazy_test_", dir=str(dest.parent)))
    t = np.arange(int(RATE * 0.5), dtype=np.float32) / RATE
    try:
        for i, key in enumerate(KEYS):
            f = 220.0 * 2 ** (i / 12)
            tone = (0.08 * np.sin(2 * np.pi * f * t) * np.exp(-t * 4)).astype(np.float32)
            write_pcm(scratch / (key + ".wav"), tone, np)
            progress("note", index=i + 1, total=21, key=key, message=f"Test captured {key}")
        (scratch / "instrument.json").write_text(json.dumps({"format": "Cazy Lyre Instrument Profile v1", "synthetic_test": True, "sample_rate": RATE, "keys": KEYS}), encoding="utf-8")
        save_profile(dest, scratch)
        progress("complete", index=21, total=21, folder=str(dest), message="Synthetic self-test complete. NOT Genshin audio.")
    finally:
        if scratch.exists():
            shutil.rmtree(scratch, ignore_errors=True)


def main():
    arg = argparse.ArgumentParser()
    arg.add_argument("--out", required=True)
    arg.add_argument("--ahk")
    arg.add_argument("--countdown", type=int, default=COUNTDOWN)
    arg.add_argument("--self-test", action="store_true")
    args = arg.parse_args()
    try:
        if args.self_test:
            self_test(Path(args.out))
        else:
            if not sys.platform.startswith("win"):
                raise RuntimeError("Live capture runs only on Windows.")
            real_capture(Path(args.out), args.ahk or "", max(5, min(30, args.countdown)))
    except Exception as e:
        progress("error", message=str(e))
        return 1
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
