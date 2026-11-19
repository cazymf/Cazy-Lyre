"""Render Cazy Lyre timed note events using locally captured 21 lyre WAVs.
No OpenAI calls, no screen capture, no live Genshin connection.
"""
from __future__ import annotations

import argparse
import json
import wave
from pathlib import Path

KEYS = set('QWERTYUASDFGHJZXCVBNM')
RATE = 48000


def load_wave(path, np):
    with wave.open(str(path), 'rb') as f:
        nch, sw, rate = f.getnchannels(), f.getsampwidth(), f.getframerate()
        frames = f.readframes(f.getnframes())
    if sw != 2 or rate != RATE or nch not in (1, 2):
        raise ValueError(f'Unexpected lyre sample format for {path.name}: {rate} Hz / {nch} channels / {sw*8} bits')
    audio = np.frombuffer(frames, dtype='<i2').reshape(-1, nch).astype(np.float32) / 32768.0
    if nch == 1:
        audio = np.repeat(audio, 2, axis=1)
    return audio


def render(events, sample_dir, out, np):
    if not events:
        raise ValueError('No notes to render')
    if len(events) > 60000:
        raise ValueError('Too many notes for sample preview')
    recorded = {key: load_wave(sample_dir / (key + '.wav'), np) for key in KEYS}
    last_ms = max(int(e.get('Ms', e.get('ms', 0))) for e in events)
    if last_ms < 0 or last_ms > 12*60*1000:
        raise ValueError('Preview duration must be between 0 and 12 minutes')
    max_tail = max(len(audio) for audio in recorded.values())
    n = int(RATE * last_ms / 1000) + max_tail + 2000
    result = np.zeros((n, 2), dtype=np.float32)
    valid = 0
    for e in events:
        start_ms = int(e.get('Ms', e.get('ms', 0)))
        if start_ms < 0:
            continue
        start = int(round(RATE * start_ms / 1000))
        chord = str(e.get('Keys', e.get('keys', ''))).upper()
        for key in set(chord):
            if key not in KEYS:
                continue
            sample = recorded[key]
            end = min(start + len(sample), n)
            if end > start:
                result[start:end, :] += sample[:end-start]
                valid += 1
    if valid == 0:
        raise ValueError('No valid lyre keys to play')
    peak = float(np.max(np.abs(result)))
    if peak > 0.95:
        result *= 0.95 / peak  # prevent clipped chords while preserving relative dynamics
    out.parent.mkdir(parents=True, exist_ok=True)
    pcm = (np.clip(result, -1, 1)*32767).astype('<i2')
    with wave.open(str(out), 'wb') as wf:
        wf.setnchannels(2)
        wf.setsampwidth(2)
        wf.setframerate(RATE)
        wf.writeframes(pcm.tobytes())
    return n / RATE, valid


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--events', type=Path, required=True)
    p.add_argument('--samples', type=Path, required=True)
    p.add_argument('--out', type=Path, required=True)
    args=p.parse_args()
    try:
        import numpy as np
        events=json.loads(args.events.read_text(encoding='utf-8'))
        duration,keys=render(events, args.samples, args.out, np)
        print(json.dumps({'status':'ok','duration_s':duration,'keys':keys,'path':str(args.out)}),flush=True)
    except Exception as e:
        print(json.dumps({'status':'error','message':str(e)}),flush=True)
        return 1
    return 0

if __name__=='__main__':
    raise SystemExit(main())
