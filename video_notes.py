#!/usr/bin/env python3
"""Cazy Lyre video transcription (local, no cloud service).

Visual mode auto-detects a Genshin 21-key overlay or a fixed-camera
Synthesia/falling-note piano keyboard. LYRE KEYS mode requires the 21-key
overlay; ordinary gameplay without a visible keyboard is not transcribed.
Produces a standard MIDI format-0 file with absolute onset times and a JSON
quality report. Video pitch calibration uses the 2-3 black-key grouping pattern.
Requires opencv-python-headless and numpy.
"""
import argparse
import json
import math
import os
import struct
import sys
from collections import Counter, defaultdict

import cv2
import numpy as np


def video_frame(cap, t, max_width=960):
    cap.set(cv2.CAP_PROP_POS_MSEC, t * 1000)
    ok, im = cap.read()
    if not ok:
        return None
    if im.shape[1] > max_width:
        h = round(im.shape[0] * max_width / im.shape[1])
        im = cv2.resize(im, (max_width, h), interpolation=cv2.INTER_AREA)
    return im



# Genshin 21-key video overlay recognition. Do not assume the overlay lives
# at any fixed screen coordinates: covers use different letterbox sizes,
# zoom, vertical offset, and overlay templates. A geometric square+diamond
# border matched across the frame calibrates the actual 3 x 7 key grid.
LYRE_OVERLAY_ROWS = ("QWERTYU", "ASDFGHJ", "ZXCVBNM")
LYRE_OVERLAY_PITCHES = (
    (72, 74, 76, 77, 79, 81, 83),
    (60, 62, 64, 65, 67, 69, 71),
    (48, 50, 52, 53, 55, 57, 59),
)


def _line_template(half):
    """Edge template for unpressed key outlines: square and tilted diamond."""
    d = int(round(1.4 * half))
    pad = d + 3
    tmpl = np.zeros((pad * 2 + 1, pad * 2 + 1), np.uint8)
    cv2.rectangle(tmpl, (pad-half, pad-half), (pad+half, pad+half), 255, 1)
    diamond = np.array([[pad,pad-d],[pad+d,pad],[pad,pad+d],[pad-d,pad]],dtype=np.int32)
    cv2.polylines(tmpl,[diamond],True,255,1)
    tmpl = cv2.GaussianBlur(tmpl,(3,3),.8)
    return tmpl.astype(np.float32)/255.,pad


def _cluster_values(values, tolerance):
    """Average near-identical key-center coordinates, no scipy dependency."""
    groups = []
    for v in sorted(values):
        if groups and abs(v - np.mean(groups[-1])) <= tolerance:
            groups[-1].append(v)
        else:
            groups.append([v])
    return [float(np.mean(group)) for group in groups]


def calibrate_lyre_overlay(img):
    """Locate the actual 3x7 key overlay geometrically.

    The old code used absolute positions measured from one screenshot, which
    failed completely on other 16:9 recordings. Here we detect 21 repeated
    square/diamond outlines and validate their 7-column/3-row lattice.
    Returns a calibration object, or None when the grid is not convincing.
    """
    if img.shape[1] > 960:
        img = cv2.resize(img,(960,round(img.shape[0]*960/img.shape[1])),interpolation=cv2.INTER_AREA)
    h,w=img.shape[:2]
    gray=cv2.cvtColor(img,cv2.COLOR_BGR2GRAY)
    edges=cv2.Canny(gray,65,140).astype(np.float32)/255
    best = None
    # Template dimensions are for 960-pixel frames. The two Genshin overlay
    # examples provided by the user calibrate near 26 and 34 pixels.
    for half in range(18,42,2):
        tmpl,pad=_line_template(half)
        if tmpl.shape[0]>=h*.35:
            continue
        corr=cv2.matchTemplate(edges,tmpl,cv2.TM_CCORR)
        # Remove regions that would put the key centers outside the frame.
        ylo=max(0,round(.19*h-pad)); yhi=min(corr.shape[0],round(.95*h-pad))
        xlo=max(0,round(.04*w-pad)); xhi=min(corr.shape[1],round(.96*w-pad))
        if ylo>=yhi or xlo>=xhi:
            continue
        region=corr[ylo:yhi,xlo:xhi]
        max_response=float(region.max())
        if max_response<35:
            continue
        dilated=cv2.dilate(region,np.ones((int(max(15,half*1.6))//2*2+1,)*2,np.uint8))
        yy,xx=np.where((region>=max_response*.55)&(region>=dilated-1e-3))
        if len(xx)<21:
            continue
        ranks=np.argsort(region[yy,xx])[::-1]
        # Accept top 21 candidates (one per overlay key). On a true overlay
        # these dominate the image because every cell has identical geometry.
        strongest=ranks[:21]
        xs=xx[strongest]+xlo+pad
        ys=yy[strongest]+ylo+pad
        xgroups=_cluster_values(xs, max(4,half*.52))
        ygroups=_cluster_values(ys, max(4,half*.52))
        if len(xgroups)!=7 or len(ygroups)!=3:
            continue
        dx=np.diff(xgroups);dy=np.diff(ygroups)
        if not (np.all(dx>half*2.4) and np.all(dx<half*4.1)
                and np.all(dy>half*2.6) and np.all(dy<half*4.2)):
            continue
        if np.std(dx)>half*.27 or np.std(dy)>half*.27:
            continue
        # Each of the 21 square/diamond outlines must have its own grid cell.
        occupancy=set()
        error=0.
        for x,y in zip(xs,ys):
            ci=int(np.argmin(abs(np.asarray(xgroups)-x)))
            ri=int(np.argmin(abs(np.asarray(ygroups)-y)))
            error+=abs(x-xgroups[ci])+abs(y-ygroups[ri])
            occupancy.add((ri,ci))
        if len(occupancy)<20:
            continue
        merit=float(np.mean(region[yy[strongest],xx[strongest]])) - error*.1
        if best is None or merit>best['merit']:
            best={'centers':[(int(round(xgroups[c])),int(round(ygroups[r])),
                               LYRE_OVERLAY_PITCHES[r][c],LYRE_OVERLAY_ROWS[r][c])
                              for r in range(3) for c in range(7)],
                  'half':half, 'merit':merit,
                  'columns':[round(v,1) for v in xgroups],
                  'rows':[round(v,1) for v in ygroups],
                  'resolution':(w,h), 'confidence':round(merit/max(1,max_response),3)}
    return best


def lyre_overlay_pressed(img, calibration):
    """Read illuminated key interiors using video-specific calibrated cells.

    The four inner corners avoid white letter labels and outlines. Bright,
    desaturated fills are far stronger than the transparent normal cells.
    """
    h,w=img.shape[:2]
    rx=max(6,int(round(calibration['half']*.65)))
    ry=rx
    # A pressed key fills the whole rectangle. Small masks at the corners
    # avoid glyphs and allow both bright-white and softer pink pulses.
    detected={}
    for cx,cy,pitch,key in calibration['centers']:
        if cx-rx<0 or cx+rx>w or cy-ry<0 or cy+ry>h:continue
        patch=img[cy-ry:cy+ry,cx-rx:cx+rx]
        hsv=cv2.cvtColor(patch,cv2.COLOR_BGR2HSV)
        xx=np.arange(2*rx)[None,:]-rx
        yy=np.arange(2*ry)[:,None]-ry
        mask=(np.abs(xx)>rx*.50)&(np.abs(yy)>ry*.50)
        v=hsv[:,:,2][mask];s=hsv[:,:,1][mask]
        bright=float(np.mean((v>174)&(s<140)))
        if bright>.32 and float(np.median(v))>154:
            detected[pitch]=(key,round(bright,3))
    return detected


def collect_lyre_overlay_notes(video_path,start_ms=0,max_seconds=0,force=False):
    cap=cv2.VideoCapture(video_path)
    if not cap.isOpened():
        raise RuntimeError('Cannot open video. Try a standard H.264 MP4.')
    fps=float(cap.get(cv2.CAP_PROP_FPS))
    count=float(cap.get(cv2.CAP_PROP_FRAME_COUNT))
    if not (math.isfinite(fps) and fps>1):
        raise RuntimeError('Video has invalid frame rate')
    duration=count/fps if count>0 else 0
    if duration>3600:
        raise RuntimeError('Video exceeds the 60-minute processing limit')
    # One clean frame is enough to establish stable key locations. Repeat
    # only when a loading screen blocks the first calibration frame.
    calibration=None
    for t in (max(1.,start_ms/1000+.3), min(4.,duration*.08), min(12.,duration*.22)):
        if .03<t<duration-.05:
            frame=video_frame(cap,t)
            if frame is not None:
                cal=calibrate_lyre_overlay(frame)
                if cal is not None:
                    calibration=cal
                    break
    if calibration is None:
        cap.release()
        if force:
            raise RuntimeError('Genshin lyre overlay could not be calibrated automatically. Ensure the 3x7 keys are visible; this video may use a different design.')
        return None
    stride=max(1,int(round(fps/60)))
    seek_ms=max(0,start_ms-180)
    cap.set(cv2.CAP_PROP_POS_MSEC,seek_ms)
    notes=[]
    active=set()
    last_ms={}
    last_on={}
    visited=0
    index=0
    # Avoid treating a one-frame bright flash as a musical press.
    while True:
        ok,frame=cap.read()
        if not ok:
            break
        index+=1
        if (index-1)%stride:
            continue
        tms=float(cap.get(cv2.CAP_PROP_POS_MSEC))
        if not math.isfinite(tms) or tms<=0:
            pos=cap.get(cv2.CAP_PROP_POS_FRAMES)
            tms=max(0.,(pos-1)*1000/fps)
        if tms<start_ms-100:
            continue
        if max_seconds and tms>start_ms+max_seconds*1000:
            break
        if frame.shape[1]>960:
            frame=cv2.resize(frame,calibration['resolution'],interpolation=cv2.INTER_AREA)
        pressed=lyre_overlay_pressed(frame,calibration)
        now=set(pressed)
        for pitch in now:
            if pitch not in active and tms-last_on.get(pitch,-1e9)>=65:
                onset=max(0,int(round(tms-10)))
                if onset>=start_ms-50:
                    notes.append((onset,pitch))
                    last_on[pitch]=tms
            last_ms[pitch]=tms
        for pitch in list(active):
            if pitch not in now and tms-last_ms.get(pitch,-1e9)>=max(30,1000/fps*1.25):
                active.remove(pitch)
        active.update(now)
        visited+=1
    cap.release()
    notes.sort()
    if len(notes)<4:
        raise RuntimeError(f'Grid located at {calibration["columns"]}, but only {len(notes)} key presses were detected. Try a higher-quality video or check the overlay brightness.')
    if len(notes)>25000:
        raise RuntimeError('Too many key detections. The key-overlay calibration might be incorrect.')
    report=dict(duration_seconds=round(duration,2),notes=len(notes),source='genshin-21-key-overlay',
                scanned_frames=visited,grid_columns=calibration['columns'],grid_rows=calibration['rows'],
                key_size=calibration['half'],grid_confidence=calibration['confidence'])
    return notes, report

def keyboard_top(img):
    """Find the bright-white piano area beginning below the dark note highway."""
    h, w = img.shape[:2]
    hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)
    white = (hsv[:, int(.045*w):int(.96*w), 2] > 145) & (hsv[:, int(.045*w):int(.96*w), 1] < 88)
    density = white.mean(axis=1).astype(np.float32)
    before = np.convolve(density, np.ones(9, np.float32)/9, mode='same')
    # at keyboard transition, brightness rises sharply and stays high.
    best = None
    for y in range(int(.46*h), int(.88*h)):
        if y-12 < 0 or y+19 >= h:
            continue
        gap = float(before[y+6:y+20].mean()-before[y-13:y-3].mean())
        white_below = float(before[y+6:y+20].mean())
        score = gap + .22*white_below
        if white_below > .36 and gap > .17 and (best is None or score > best[0]):
            best = (score, y)
    if best is None:
        return None
    return best[1]


def dark_segments(img, y):
    h, w = img.shape[:2]
    y = min(h-2, max(0, y))
    # Blur across 3 rows to avoid a small amount of video interference.
    strip = cv2.cvtColor(img[max(0,y-2):min(h,y+3)], cv2.COLOR_BGR2GRAY)
    med = np.median(strip, axis=0)
    dark = (med < 82)
    # Convert dark horizontal runs to candidate black-key x positions.
    runs = []
    start = None
    for i in range(w+1):
        on = i<w and bool(dark[i])
        if on and start is None:
            start=i
        if not on and start is not None:
            width=i-start
            if max(3, w*.005) < width < w*.04:
                runs.append(((i+start)/2., width))
            start = None
    return runs


def locate_c4(img, top):
    """Return approximate center of C4 and a white-key step in pixels.

    Black keys are arranged C#,D#, [gap], F#,G#,A#, [gap]. We fit that
    repeating signature rather than guessing equal chromatic spacing.
    """
    h,w=img.shape[:2]
    for rel in [.30,.18,.48,.09]:
        y=round(top+(h-top)*rel)
        pairs=dark_segments(img,y)
        if len(pairs)<12:
            continue
        centers=np.array([p[0] for p in pairs], np.float32)
        spacing=np.diff(centers)
        spacing=spacing[(spacing>max(7,w*.011))&(spacing<w*.095)]
        if len(spacing)<8:
            continue
        # Min spacing is one white-key width between neighboring accidentals.
        step=float(np.quantile(spacing,.28))
        if not w/85 < step < w/14:
            continue
        candidates=[]
        for i in range(1,len(centers)-2):
            dprev=float(centers[i]-centers[i-1])/step
            dnext=float(centers[i+1]-centers[i])/step
            dnext2=float(centers[i+2]-centers[i+1])/step
            # Long gap before C# / short C#->D# / long D#->F#.
            err=(dprev-2)**2+(dnext-1)**2+(dnext2-2)**2
            if err < 1.1:
                c=float(centers[i]-.5*step)
                candidates.append((c,err))
        if not candidates:
            continue
        # In mainstream full-screen tutorials, middle C is near the center.
        # Octave ambiguity is resolved by making C4 nearest 44% of width.
        selected=min(candidates,key=lambda c: abs(c[0]-.44*w)/step +.05*c[1])
        return selected[0],step,len(pairs)
    return None


def key_positions(c4,step,w):
    natural_offsets=[(0,0),(2,1),(4,2),(5,3),(7,4),(9,5),(11,6)]
    black_offsets=[(1,.5),(3,1.5),(6,3.5),(8,4.5),(10,5.5)]
    pitches=[]
    for octv in range(-5,6):
        for semi,white in natural_offsets+black_offsets:
            x=c4+(octv*7+white)*step
            if -step < x < w+step:
                pitches.append((x,60+octv*12+semi))
    pitches.sort()
    return pitches


def collect_notes(video_path,start_ms=0, target_fps=30, calibration=None, max_seconds=0):
    cap=cv2.VideoCapture(video_path)
    if not cap.isOpened():
        raise RuntimeError('Cannot open MP4. Try an H.264 video or switch to Audio mode.')
    fps=float(cap.get(cv2.CAP_PROP_FPS))
    count=float(cap.get(cv2.CAP_PROP_FRAME_COUNT))
    if not (fps>1 and math.isfinite(fps)):
        raise RuntimeError('Video has invalid frame rate')
    duration=count/fps if count>0 else 0
    if duration>3600:
        raise RuntimeError('Video is longer than the 60-minute processing limit')
    # Try frames away from loading screens or titles; support starting late.
    sample_times=[]
    for t in [max(2.,start_ms/1000+1), min(12,duration*.12), min(25,duration*.3),min(43,duration*.55)]:
        if .1<t<duration-1 and t not in sample_times:
            sample_times.append(t)
    chosen=None
    for t in sample_times:
        frame=video_frame(cap,t)
        if frame is None:
            continue
        top=keyboard_top(frame)
        if top is None:
            continue
        cal=locate_c4(frame,top)
        if cal is None:
            continue
        # Prefer a convincing C4 anchor near the usual middle-C location.
        # Some highlighted keys can temporarily mask one black-key group;
        # do not let a one-octave-shifted calibration win on key count alone.
        black_count=len(dark_segments(frame,round(top+(frame.shape[0]-top)*.3)))
        center_bias=abs(cal[0]-.44*frame.shape[1])/max(1,cal[1])
        merit=center_bias + max(0, 27-black_count)*.04
        if chosen is None or merit<chosen[4]:
            chosen=(top,cal,black_count,frame.shape[:2],merit)
    if chosen is None:
        cap.release()
        raise RuntimeError('No calibrated Synthesia piano keyboard found. Visual mode needs a stable on-screen piano and colored falling notes; choose Audio mode for ordinary videos.')
    top,(c4,step,n_black),_,size,_=chosen
    if calibration is not None:
        c4=calibration
    h,w=size
    keys=key_positions(c4,step,w)
    # For every pressed note, use the exact physical x of its column.
    xcols=[]
    for x,p in keys:
        if 0<=x<w and 24<=p<=108:
            xcols.append((round(x),p))
    # Scan a few pixels above the top of the keyboard where falling bars land.
    y1=max(3,top-max(5,round(h*.018)))
    y2=max(y1+3,top-max(1,round(h*.004)))
    seek_ms=max(0,start_ms-220)
    cap.set(cv2.CAP_PROP_POS_MSEC,seek_ms)
    cap_fps=fps
    stride=max(1,round(cap_fps/target_fps))
    frame_idx=int(round(seek_ms*fps/1000))
    active_since={}
    last_seen={}
    last_note={}
    notes=[]
    visited=0
    while True:
        ok,frame=cap.read()
        if not ok:
            break
        frame_idx+=1
        if frame_idx%stride:
            continue
        if max_seconds and (frame_idx/fps)>start_ms/1000+max_seconds:
            break
        if frame.shape[1]>960:
            frame=cv2.resize(frame,(w,h),interpolation=cv2.INTER_AREA)
        if frame.shape[:2] !=(h,w):
            continue
        tms=float(cap.get(cv2.CAP_PROP_POS_MSEC))
        if tms<=0:
            tms=(frame_idx-1)*1000/fps
        if tms<start_ms-100:
            continue
        hsv=cv2.cvtColor(frame[y1:y2,:,:],cv2.COLOR_BGR2HSV)
        # Glowing note blocks have strong saturation and value; group by
        # horizontal extent to reject single-pixel aliasing and backgrounds.
        mask=((hsv[:,:,1]>85)&(hsv[:,:,2]>115)).sum(axis=0)>=max(2,(y2-y1)//3)
        # Morphological closing repairs a 1px centerline inside the note.
        if mask.size>=3:
            mask=np.maximum(mask, np.r_[False,mask[:-1]])
        runs=[]
        st=None
        for xx in range(w+1):
            on=xx<w and bool(mask[xx])
            if on and st is None:st=xx
            if not on and st is not None:
                wid=xx-st
                if step*.24 < wid < step*1.38:
                    runs.append(((st+xx-1)/2.,wid))
                st=None
        detected={}
        for center,width in runs:
            # Use pitch with minimum horizontal distance to the known key
            # center, but reject distant noise from non-piano animation.
            x,p=min(keys, key=lambda k:abs(k[0]-center))
            distance=abs(x-center)
            if distance>step*.36 or p<24 or p>108:continue
            # Bars that span entire scene tend to be much wider than keys.
            if p not in detected or distance<detected[p]:
                detected[p]=distance
        # Rising edges trigger note-on; two missed sampled frames release
        # the note. Repeated notes still work after a short visible gap.
        for p in detected:
            if p not in active_since:
                if tms-last_note.get(p,-1e9)>62:
                    # A scanned note occurs slightly before its visual bar
                    # reaches the key line (usually under 40ms).
                    onset=max(0,int(round(tms-12)))
                    if onset>=start_ms-65:
                        notes.append((onset,int(p)))
                        last_note[p]=tms
                active_since[p]=tms
            last_seen[p]=tms
        for p in list(active_since):
            if p not in detected and tms-last_seen.get(p,-1e9)>52:
                del active_since[p]
        visited+=1
    cap.release()
    # Discard accidental isolated screen elements / short empty clips.
    notes.sort()
    clean=[]
    last={}
    for ms,p in notes:
        if ms-last.get(p,-1000)<75:continue
        clean.append((ms,p))
        last[p]=ms
    if len(clean)<12:
        raise RuntimeError(f'Only {len(clean)} visual notes found. This video may not show readable falling notes; try Audio mode or a clearer Synthesia video.')
    if len(clean)>25000:
        raise RuntimeError('Excessive visual detections; video layout calibration failed. Try Audio mode.')
    return clean, dict(duration_seconds=round(duration,2),keyboard_y=top,c4_x=round(c4,2),white_key_width=round(step,2),black_keys=n_black,notes=len(clean),source='video-cv', scanned_frames=visited)


def vlq(value):
    value=int(value)
    out=[value&0x7f]
    value>>=7
    while value:
        out.append((value&0x7f)|0x80)
        value>>=7
    return bytes(reversed(out))


def write_midi(events,path):
    # 480 ticks/qn and 500,000 usec/qn => 960 ticks/sec.
    track=bytearray()
    prev=0
    transmissions=[]
    for ms,p in events:
        tick=round(ms*0.96)
        transmissions.append((tick,1,p))
        transmissions.append((tick+max(70,int(120*.96)),0,p))
    transmissions.sort(key=lambda s:(s[0],s[1],s[2]))
    for tick,is_on,p in transmissions:
        track+=vlq(max(0,tick-prev))
        track+=bytes([0x90 if is_on else 0x80, int(p), 94 if is_on else 0])
        prev=tick
    track+=b'\x00\xff\x2f\x00'
    header=b'MThd'+struct.pack('>IHHH',6,0,1,480)
    chunk=b'MTrk'+struct.pack('>I',len(track))+track
    with open(path,'wb') as f: f.write(header+chunk)


def extract_audio(video_path,output_wav):
    try:
        import imageio_ffmpeg
    except ImportError as e:
        raise RuntimeError('Video audio extraction needs imageio-ffmpeg. Click SETUP VIDEO ENGINE.') from e
    import subprocess
    command=[imageio_ffmpeg.get_ffmpeg_exe(), '-nostdin','-hide_banner','-loglevel','error', '-y', '-i',video_path,'-vn','-ac','1','-ar','22050',output_wav]
    try:
        subprocess.run(command,check=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=1200)
    except subprocess.CalledProcessError as e:
        raise RuntimeError('Cannot extract audio from this MP4: '+e.stderr.decode('utf-8','replace')[-400:])


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--input',required=True)
    parser.add_argument('--outdir',required=True)
    parser.add_argument('--mode',choices=('visual','overlay','audio','hybrid'),default='visual')
    parser.add_argument('--start-ms',type=int,default=0)
    parser.add_argument('--max-seconds',type=float,default=0,help='testing only')
    args=parser.parse_args()
    os.makedirs(args.outdir,exist_ok=True)
    details={'mode':args.mode}
    if args.mode in ('visual','hybrid','overlay'):
        overlay=collect_lyre_overlay_notes(args.input,max(0,args.start_ms),max_seconds=args.max_seconds,force=args.mode=='overlay')
        if overlay is not None:
            notes,stats=overlay
        else:
            notes,stats=collect_notes(args.input,max(0,args.start_ms),max_seconds=args.max_seconds)
        midi=os.path.join(args.outdir,'visual.mid')
        write_midi(notes,midi)
        details.update(stats)
    if args.mode in ('audio','hybrid'):
        wav=os.path.join(args.outdir,'audio.wav')
        extract_audio(args.input,wav)
        details['audio_wav']='audio.wav'
    with open(os.path.join(args.outdir,'video_report.json'),'w',encoding='utf-8') as f:
        json.dump(details,f,ensure_ascii=False,indent=2)
    print(json.dumps(details,ensure_ascii=False))


if __name__=='__main__':
    try:
        main()
    except Exception as e:
        print(str(e),file=sys.stderr)
        sys.exit(1)
