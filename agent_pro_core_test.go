package main

import (
    "encoding/json"
    "strings"
    "testing"
)
func TestAgentBudgetModes(t *testing.T) {
    if normalizeLyreArrangeMode(lyreArrangeAgent)!=lyreArrangeAgent { t.Fatal("agent mode not preserved") }
    if normalizeAgentBudget(999)!=100 {t.Fatal("invalid budget did not default to 100 cents")}
    if nextAgentBudget(50)!=100 || nextAgentBudget(100)!=200 || nextAgentBudget(200)!=50 {t.Fatal("budget cycling incorrect")}
    meter:=newAgentMeter(50)
    if !meter.canCall(strings.Repeat("a",500),strings.Repeat("b",500),1500) {t.Fatal("small call unexpectedly rejected")}
    meter.SpentUSD=0.49
    if meter.canCall(strings.Repeat("a",500),strings.Repeat("b",500),5500) {t.Fatal("over-budget call was allowed")}
    if !meter.MaxReached {t.Fatal("budget limit wasn't flagged")}
}
func TestSolTokenEstimate(t *testing.T) {
    estimate:=estimateSolUSD(1000000,1000000,0)
    if estimate<11.9999 || estimate>12.0001 {t.Fatalf("expected 12 USD, got %f",estimate)}
    cached:=estimateSolUSD(1000000,0,1000000)
    if cached<0.0999 || cached>0.1001 {t.Fatalf("expected 0.1 USD, got %f",cached)}
}
func TestAgentRebuildSectionsAndChordTiming(t *testing.T) {
    chunks:=[][]lyreEvent{
        {{Ms:100,Keys:"a"},{Ms:200,Keys:"s"},{Ms:300,Keys:"d"}},
        {{Ms:400,Keys:"f"},{Ms:500,Keys:"g"}},
    }
    sections:=[][]lyreEvent{
        {{Ms:0,Keys:"aq"},{Ms:100,Keys:"sd"},{Ms:100,Keys:"sd"}},
        originalRelativeChunk(chunks[1]),
    }
    out:=reconstructSections(chunks,sections)
    if len(out)!=4 {t.Fatalf("expected 4 nonduplicate note events, got %d: %+v",len(out),out)}
    if out[0].Ms!=100 || out[0].Keys!="aq" || out[2].Ms!=400 || out[3].Ms!=500 {t.Fatalf("incorrect absolute timestamps: %+v",out)}
}
func TestAgentDirectorAndCriticDataGrounded(t *testing.T) {
    score:=lyreScore{DurationMs:3000,Events:[]lyreEvent{{Ms:0,Keys:"a"},{Ms:200,Keys:"s"},{Ms:500,Keys:"d"}}}
    ref:=lyreSourceBundle{PianoNotes:[]pianoEvent{{Ms:0,Pitch:60,Velocity:90}}}
    summary:=summarizeScoreForDirector(score,ref)
    var js map[string]interface{}
    if err:=json.Unmarshal([]byte(summary),&js);err!=nil{t.Fatal(err)}
    if js["event_count"].(float64)!=3 || js["transcribed_midi_notes"].(float64)!=1 {t.Fatal("source summary missing note evidence")}
    critic:=buildAgentCriticPrompt("Test",agentDirectorPlan{Melody:"Ascending"},score.Events,score.Events,[][]lyreEvent{score.Events})
    if !strings.Contains(critic,"lyre_key_map") && !strings.Contains(critic,"Z=48") {t.Fatal("critic missing the exact key pitch map")}
}
func TestAgentAHKExportPreservesControlsAndMusic(t *testing.T) {
    score:=lyreScore{Events:[]lyreEvent{
        {Ms:0,Keys:"aq"},{Ms:200,Keys:"sd"},{Ms:400,Keys:"f"},{Ms:600,Keys:"j"},
    },Notes:4,DurationMs:600,Quality:qualityFull}
    source:=generateLyreAHK("Agent Pro Smoke Test",score)
    if err:=validateNonemptyAHK(source);err!=nil {t.Fatalf("AHK invalid: %v",err)}
    if !strings.Contains(source,"XButton1") || !strings.Contains(source,"XButton2") {t.Fatal("mouse controls missing")}
    if strings.Contains(source,"Esc::") || strings.Contains(source,"Escape::") {t.Fatal("Esc must never be bound")}
    if !strings.Contains(source,`"aq"`) {t.Fatal("chord data missing from generated AHK")}
}
