//go:build windows

package main

import (
    "bytes"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "time"
    "strings"
)

// Agent Pro is a locally orchestrated, multi-pass music-arranging workflow.
// It never writes AI-generated AHK; AI can only return bounded lyre-note data.
// The original audio is not passed to the API; reference MIDI comes from the
// real local transcription and is explicitly labeled as potentially noisy.
const agentModel = "gpt-6.1-sol"

type agentUsage struct {
    InputTokens int `json:"input_tokens"`
    OutputTokens int `json:"output_tokens"`
    InputTokensDetails struct { CachedTokens int `json:"cached_tokens"` } `json:"input_tokens_details"`
}
type agentEnvelope struct {
    Status string `json:"status"`
    Output []apiOutput `json:"output"`
    Usage agentUsage `json:"usage"`
    IncompleteDetails struct { Reason string `json:"reason"` } `json:"incomplete_details"`
    Error *struct { Message string `json:"message"` } `json:"error"`
}
func agentJSONSchema(name string, inner map[string]interface{}) map[string]interface{} {
    return map[string]interface{}{
        "format": map[string]interface{}{
            "type":"json_schema", "name":name, "strict":true, "schema":inner,
        },
    }
}
func agentStringField() map[string]interface{} { return map[string]interface{}{"type":"string"} }
func agentIntField() map[string]interface{} { return map[string]interface{}{"type":"integer"} }
func agentObject(fields map[string]interface{}, required ...string) map[string]interface{} {
    return map[string]interface{}{"type":"object", "additionalProperties":false, "properties":fields, "required":required}
}
func agentScoreSchema() map[string]interface{} {
    item := agentObject(map[string]interface{}{"ms":agentIntField(), "keys":agentStringField()}, "ms", "keys")
    return agentObject(map[string]interface{}{"events":map[string]interface{}{"type":"array", "items":item}}, "events")
}
func agentPlanSchema() map[string]interface{} {
    f:=map[string]interface{}{"tonality":agentStringField(),"melody":agentStringField(),"harmony":agentStringField(),"rhythm":agentStringField(),"arrangement":agentStringField()}
    return agentObject(f,"tonality","melody","harmony","rhythm","arrangement")
}
func agentCriticSchema() map[string]interface{} {
    rev := agentObject(map[string]interface{}{"section":agentIntField(),"reason":agentStringField()}, "section","reason")
    return agentObject(map[string]interface{}{"summary":agentStringField(),"revisions":map[string]interface{}{"type":"array","items":rev}}, "summary","revisions")
}
func agentCall(key, prompt, instructions, schemaName, effort string, schema map[string]interface{}, maxOutput int, m *agentMeter) (string,error) {
    if !m.canCall(prompt,instructions,maxOutput) { return "",errors.New("per-song budget guard reached before another AI call") }
    data := map[string]interface{}{
        "model":agentModel,
        "input":prompt,
        "instructions":instructions,
        "reasoning":map[string]string{"effort":effort},
        "max_output_tokens":maxOutput,
        "text":agentJSONSchema(schemaName,schema),
        "store":false,
    }
    b,e:=json.Marshal(data); if e!=nil{return "",e}
    req,e:=http.NewRequest("POST","https://api.openai.com/v1/responses",bytes.NewReader(b)); if e!=nil { return "",e }
    req.Header.Set("Content-Type","application/json")
    req.Header.Set("Authorization","Bearer "+key)
    client:=&http.Client{Timeout:150*time.Second}
    resp,e:=client.Do(req); if e!=nil{return "",fmt.Errorf("Agent Pro API request: %w",e)}
    defer resp.Body.Close()
    body,e:=io.ReadAll(io.LimitReader(resp.Body,3<<20)); if e!=nil{return "",e}
    if resp.StatusCode<200 || resp.StatusCode>=300 {
        var problem apiErrorEnvelope
        if json.Unmarshal(body,&problem)==nil && problem.Error.Message!="" { return "",fmt.Errorf("OpenAI HTTP %d: %s", resp.StatusCode,problem.Error.Message) }
        return "",fmt.Errorf("OpenAI HTTP %d",resp.StatusCode)
    }
    var out agentEnvelope
    if e:=json.Unmarshal(body,&out);e!=nil{return "",fmt.Errorf("decode Agent Pro response: %w",e)}
    // Count billed tokens even when a successful HTTP response is incomplete.
    spent := estimateSolUSD(out.Usage.InputTokens,out.Usage.OutputTokens,out.Usage.InputTokensDetails.CachedTokens)
    if out.Usage.InputTokens==0 && out.Usage.OutputTokens==0 {
        spent=estimateSolUSD((len(prompt)+len(instructions))/3+1000,maxCreativeInt(len(body)/3,300),0)
    }
    m.SpentUSD+=spent
    m.Calls++
    if out.Error!=nil {return "",errors.New(out.Error.Message)}
    if out.Status=="incomplete" {return "",fmt.Errorf("model response incomplete: %s",out.IncompleteDetails.Reason)}
    var textParts []string
    for _, block:=range out.Output {
        if block.Type=="message" {for _, content:=range block.Content {
            if content.Type=="output_text" {textParts=append(textParts,content.Text)}
        }}
    }
    result:=strings.TrimSpace(strings.Join(textParts,"\n"))
    if result=="" {return "",errors.New("Agent Pro returned no output")}
    return result,nil
}

func createAgentProArrangement(key,title string, initial lyreScore, bundle lyreSourceBundle, cents int, report func(string)) (lyreScore,error) {
    if len(initial.Events)<4 {return lyreScore{},errors.New("no music events to arrange")}
    chunks:=chunkCreativeScore(initial.Events, creativeChunkSize)
    if len(chunks)>28 {return lyreScore{},errors.New("song is too long for Agent Pro; choose a shorter section")}
    meter:=newAgentMeter(cents)
    if report!=nil {report(fmt.Sprintf("Agent Pro: Director → Arranger → Critic using GPT-6.1 Sol; estimated per-song budget guard $%.2f.",meter.BudgetUSD))}
    const directorInstruction=`You are a professional music director, pianist and Genshin Windsong Lyre arranger. Analyze only the supplied transcription data; you cannot hear the original MP3 or watch the video. Identify likely melody, harmonic movement, groove, repeating motifs and source noise. Give an actionable global arranging plan. Don't claim knowledge of the original by title alone. Preserve musical identity rather than creating random notes. Output exactly the requested JSON schema.`
    directorPrompt:="Create a global musical direction plan for: "+title+". Interpret integer MIDI pitches with their real pitch meaning. Help the next model convert this into a playable 21-key natural-scale lyre arrangement. Distinguish accompaniment from melody; avoid arbitrary octave folding. Source transcription may be noisy.\n\n"+summarizeScoreForDirector(initial,bundle)
    planText,e:=agentCall(key,directorPrompt,directorInstruction,"lyre_director", "medium",agentPlanSchema(),3600,&meter)
    if e!=nil{return lyreScore{},fmt.Errorf("Agent Pro Director: %w",e)}
    var plan agentDirectorPlan
    if e=json.Unmarshal([]byte(stripCreativeCodeFence(planText)),&plan);e!=nil{return lyreScore{},fmt.Errorf("Agent Pro Director returned invalid plan: %w",e)}
    if plan.Arrangement=="" {return lyreScore{},errors.New("Agent Pro Director returned an empty plan")}
    if report!=nil {report(fmt.Sprintf("Director plan ready: %s (estimated API use $%.3f).",truncateAgentStatus(plan.Melody,76),meter.SpentUSD))}

    sections:=make([][]lyreEvent,len(chunks))
    kept:=0; repaired:=0
    const arrangerInstruction = creativeArrangementInstructions+`\nYou are now the ARRANGER stage, following a global musical-director plan. Use the real pitch map provided and the musical context around each chunk. Preserve identifiable motifs across chunk boundaries. Do not add random notes based on the song title. Make the accompaniment support the melody instead of obscuring it. Output only the strict JSON note-events schema.`
    for i,ch:=range chunks {
        originalPrompt,err:=buildCreativeChunkPrompt(title,"Use the Director plan to build a coherent, expressive Genshin Lyre arrangement that preserves the lead line and musical phrasing.",ch,i+1,len(chunks),bundle)
        if err!=nil{return lyreScore{},err}
        contextBefore:=adjacentContext(chunks,i-1,true)
        contextAfter:=adjacentContext(chunks,i+1,false)
        prompt := "GLOBAL DIRECTOR PLAN: "+planText+"\n"+agentKeyMap+"\nPRECEDING ORIGINAL PHRASE (context only): "+contextBefore+"\nFOLLOWING ORIGINAL PHRASE (context only): "+contextAfter+"\n\n"+originalPrompt
        if report!=nil {report(fmt.Sprintf("Agent Pro Arranger %d/%d; estimated spend $%.3f / $%.2f.",i+1,len(chunks),meter.SpentUSD,meter.BudgetUSD))}
        response,err:=agentCall(key,prompt,arrangerInstruction,"lyre_agent_arrangement","medium",agentScoreSchema(),5600,&meter)
        if err!=nil {
            if meter.MaxReached {
                if report!=nil {report(fmt.Sprintf("Agent Pro budget guard reached; remaining %d section(s) retain original notes.",len(chunks)-i))}
                for j:=i;j<len(chunks);j++ {sections[j]=originalRelativeChunk(chunks[j]);kept++}
                break
            }
            return lyreScore{},fmt.Errorf("Agent Pro section %d/%d: %w",i+1,len(chunks),err)
        }
        span:=ch[len(ch)-1].Ms-ch[0].Ms
        result,rep,fallback,reason:=chooseCreativeSection(response,ch,span)
        if fallback {kept++;if report!=nil{report(fmt.Sprintf("Section %d retained source: %s",i+1,truncateAgentStatus(reason,75)))}}
        repaired+=rep
        sections[i]=result
    }
    if kept==len(chunks) {return lyreScore{},errors.New("Agent Pro couldn't improve any section within the budget; nothing was exported")}
    candidate:=reconstructSections(chunks,sections)
    if len(candidate)<4 {return lyreScore{},errors.New("Agent Pro produced too few valid musical notes")}
    // Critic reviews MIDI notes and entire arrangement as symbolic music, not audio.
    const criticInstruction=`You are an exacting piano-arrangement critic. You cannot hear audio and must not pretend you can. Review the original and arranged note events for awkward melody jumps, chord crowding, unexplained pauses, mismatched phrases, and structural discontinuities. Select at most TWO sections that actually need a second pass. Do not request revisions without a clear reason. Use section numbers 1-based. Return only the requested JSON schema.`
    critiquePrompt:=buildAgentCriticPrompt(title,plan,initial.Events,candidate,chunks)
    reviewText,reviewErr:=agentCall(key,critiquePrompt,criticInstruction,"lyre_critic","medium",agentCriticSchema(),3000,&meter)
    if reviewErr==nil {
        var review agentCriticResult
        if json.Unmarshal([]byte(stripCreativeCodeFence(reviewText)),&review)==nil {
            if report!=nil {report("Agent Pro Critic: "+truncateAgentStatus(review.Summary,115))}
            reviewed:=make(map[int]bool)
            revisions:=0
            for _,issue:=range review.Revisions {
                ix:=issue.Section-1
                if ix<0 || ix>=len(chunks) || reviewed[ix] || revisions>=2 {continue}
                reviewed[ix]=true; revisions++
                previous:=sections[ix]
                ch:=chunks[ix]
                base,err:=buildCreativeChunkPrompt(title,"Revise the melody and harmony based on Critic feedback without losing the distinctive tune.",ch,ix+1,len(chunks),bundle)
                if err!=nil {continue}
                prevBytes,_:=json.Marshal(previous)
                revisePrompt:= "DIRECTOR PLAN: "+planText+"\nCRITIC FEEDBACK FOR THIS SECTION: "+issue.Reason+"\nFIRST DRAFT NOTE EVENTS (relative ms): "+string(prevBytes)+"\nREVISE ONLY THE SECTION BELOW. Preserve recognizable phrase boundaries, keep every key valid and try to resolve the critic's concrete issue.\n"+base
                if report!=nil {report(fmt.Sprintf("Agent Pro polish pass: revising section %d (%s)",issue.Section,truncateAgentStatus(issue.Reason,80)))}
                response,err:=agentCall(key,revisePrompt,arrangerInstruction,"lyre_agent_polish","medium",agentScoreSchema(),5600,&meter)
                if err!=nil {if meter.MaxReached {break};if report!=nil{report("Polish pass skipped: "+err.Error())};continue}
                span:=ch[len(ch)-1].Ms-ch[0].Ms
                polished,rep,fallback,_:=chooseCreativeSection(response,ch,span)
                if !fallback && len(polished)>=maxCreativeInt(3,len(ch)/3) {sections[ix]=polished;repaired+=rep}
            }
        }
    } else if report!=nil {report("Critic pass skipped: "+truncateAgentStatus(reviewErr.Error(),120))}
    candidate=reconstructSections(chunks,sections)
    if len(candidate)<maxCreativeInt(4,len(initial.Events)/4) {return lyreScore{},errors.New("Agent Pro removed too many note events")}
    result:=initial
    result.Events=candidate
    result.Notes=len(candidate)
    result.Chords=0
    for _,ev:=range candidate {if len(ev.Keys)>1 {result.Chords++}}
    result.DurationMs=candidate[len(candidate)-1].Ms
    result.CreativeKeptSections=kept
    result.CreativeChordRepairs=repaired
    result.AgentSpentUSD=meter.SpentUSD
    result.AgentCalls=meter.Calls
    if report!=nil {report(fmt.Sprintf("Agent Pro finished: %d events, %d AI calls, approx $%.3f used; %d original sections preserved. PREVIEW before export.",len(candidate),meter.Calls,meter.SpentUSD,kept))}
    return result,nil
}

