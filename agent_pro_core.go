package main

import (
  "encoding/json"
  "math"
  "sort"
  "strings"
)

const agentKeyMap = "Windsong Lyre pitches (MIDI values): Z=48 C3, X=50 D3, C=52 E3, V=53 F3, B=55 G3, N=57 A3, M=59 B3; A=60 C4, S=62 D4, D=64 E4, F=65 F4, G=67 G4, H=69 A4, J=71 B4; Q=72 C5, W=74 D5, E=76 E5, R=77 F5, T=79 G5, Y=81 A5, U=83 B5. These are the ONLY playable pitches. Adjacent letters in a chord mean simultaneous presses."

type agentMeter struct {
    BudgetUSD float64
    SpentUSD float64
    Calls int
    MaxReached bool
}
func newAgentMeter(cents int) agentMeter {
    return agentMeter{BudgetUSD: float64(normalizeAgentBudget(cents))/100.0}
}
func estimateSolUSD(input, output, cached int) float64 {
    if input < 0 { input = 0 }
    if output < 0 { output = 0 }
    if cached < 0 { cached = 0 }
    if cached > input { cached = input }
    // Current standard pricing, per 1M tokens. Estimates exclude platform extras.
    return (float64(input-cached)*2.0 + float64(cached)*0.1 + float64(output)*10.0)/1000000
}
func (m *agentMeter) canCall(prompt, instructions string, maxOutput int) bool {
    // Conservative request-size reservation (reasoning tokens are included in output).
    forecast := estimateSolUSD((len(prompt)+len(instructions))/3+1200, maxOutput, 0)
    if m.SpentUSD + forecast > m.BudgetUSD {
        m.MaxReached = true
        return false
    }
    return true
}

func summarizeScoreForDirector(score lyreScore, bundle lyreSourceBundle) string {
    type directorSummary struct {
        DurationMs int64 `json:"duration_ms"`
        Events int `json:"event_count"`
        SourcePitchCount int `json:"transcribed_midi_notes"`
        LyreKeys string `json:"lyre_key_map"`
        Notes []lyreEvent `json:"events_sample"`
        MIDI []pianoEvent `json:"midi_sample,omitempty"`
        Bins []int `json:"events_per_10_seconds"`
    }
    copyEvents:=score.Events
    if len(copyEvents)>900 {
        sample:=make([]lyreEvent,0,900)
        for i:=0;i<len(copyEvents);i+=int(math.Ceil(float64(len(copyEvents))/900.0)) { sample=append(sample,copyEvents[i]) }
        copyEvents=sample
    }
    bins:=make([]int, 1+int(score.DurationMs/10000)); if len(bins)>180 {bins=bins[:180]}
    for _,n:=range score.Events {idx:=int(n.Ms/10000); if idx>=0 && idx<len(bins) {bins[idx]++}}
    sampleMIDI:=bundle.PianoNotes
    if len(sampleMIDI)>900 {
        step:=int(math.Ceil(float64(len(sampleMIDI))/900.0))
        out:=make([]pianoEvent,0,900)
        for i:=0;i<len(sampleMIDI);i+=step {out=append(out,sampleMIDI[i])}
        sampleMIDI=out
    }
    b,_:=json.Marshal(directorSummary{score.DurationMs,len(score.Events),len(bundle.PianoNotes),agentKeyMap,copyEvents,sampleMIDI,bins})
    return string(b)
}

type agentDirectorPlan struct { Tonality string `json:"tonality"`; Melody string `json:"melody"`; Harmony string `json:"harmony"`; Rhythm string `json:"rhythm"`; Arrangement string `json:"arrangement"` }
type agentCriticNote struct {Section int `json:"section"`; Reason string `json:"reason"`}
type agentCriticResult struct {Summary string `json:"summary"`; Revisions []agentCriticNote `json:"revisions"`}


func truncateAgentStatus(s string,n int) string {
    s=strings.ReplaceAll(strings.ReplaceAll(s,"\n"," "),"\r"," ")
    if len(s)>n {return s[:n]+"..."}
    return s
}
func originalRelativeChunk(ch []lyreEvent) []lyreEvent {
    out:=make([]lyreEvent,len(ch));if len(ch)==0{return out}
    for i,e:=range ch {out[i]=lyreEvent{Ms:e.Ms-ch[0].Ms,Keys:e.Keys}}
    return out
}
func adjacentContext(chunks [][]lyreEvent,index int,before bool) string {
    if index<0 || index>=len(chunks) {return "none"}
    ch:=chunks[index]
    if len(ch)>8 {
        if before {ch=ch[len(ch)-8:]}else{ch=ch[:8]}
    }
    b,_:=json.Marshal(ch);return string(b)
}
func reconstructSections(chunks [][]lyreEvent, sections [][]lyreEvent) []lyreEvent {
    out:=make([]lyreEvent,0)
    for i,ch:=range chunks {
        if len(ch)==0 {continue}
        section:=sections[i]
        if len(section)==0 {section=originalRelativeChunk(ch)}
        for _,e:=range section {out=append(out,lyreEvent{Ms:ch[0].Ms+e.Ms,Keys:e.Keys})}
    }
    sort.SliceStable(out,func(i,j int)bool{return out[i].Ms<out[j].Ms})
    unique:=make([]lyreEvent,0,len(out))
    for _,e:=range out {
        if e.Ms<0 || e.Keys=="" {continue}
        if len(unique)>0 && unique[len(unique)-1].Keys==e.Keys && e.Ms-unique[len(unique)-1].Ms<18 {continue}
        unique=append(unique,e)
    }
    return unique
}
func buildAgentCriticPrompt(title string, plan agentDirectorPlan, original,candidate []lyreEvent,chunks [][]lyreEvent) string {
    compact:=func(events []lyreEvent,limit int) []lyreEvent {
        if len(events)<=limit {return events}
        out:=make([]lyreEvent,0,limit);step:=int(math.Ceil(float64(len(events))/float64(limit)))
        for i:=0;i<len(events);i+=step {out=append(out,events[i])}
        return out
    }
    ranges:=make([]map[string]interface{},0,len(chunks))
    for i,ch:=range chunks {
        if len(ch)>0 {ranges=append(ranges,map[string]interface{}{"section":i+1,"start_ms":ch[0].Ms,"end_ms":ch[len(ch)-1].Ms})}
    }
    data,_:=json.Marshal(map[string]interface{}{"song":title,"pitch_map":agentKeyMap,"director_plan":plan,"sections":ranges,"source_note_sample":compact(original,850),"candidate_note_sample":compact(candidate,850)})
    return "Critique the WHOLE arranged transcription as symbolic Genshin Lyre music. Find up to 2 material problems worth revising; otherwise provide an empty revisions array. Evaluate continuity and readability, not imaginary audio quality.\n"+string(data)
}
