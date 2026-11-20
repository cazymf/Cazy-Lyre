package main

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

// Song Sync uses an independent, versioned data feed; it never changes application binaries.
// The canonical feed is github.com/cazymf/Cazy-Lyre/main/song-library/.
type songCatalog struct {
    Format int `json:"format"`
    Songs []songEntry `json:"songs"`
}
type songEntry struct {
    ID string `json:"id"`
    Title string `json:"title"`
    SHA256 string `json:"sha256"`
}
type importedSong struct { Path, Title string }
type songSyncReport struct { Imported []importedSong; Downloaded int; Unchanged int }

var songIDPattern=regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)
var songHashPattern=regexp.MustCompile(`^[a-f0-9]{64}$`)

func parseSongCatalog(src []byte)(songCatalog,error) {
    var out songCatalog
    if err:=json.Unmarshal(src,&out);err!=nil {return out,err}
    if out.Format!=1 || len(out.Songs)>200 {return out,errors.New("unsupported song catalog format or size")}
    seen:=map[string]bool{}
    for _,song:=range out.Songs {
        if !songIDPattern.MatchString(song.ID) || !songHashPattern.MatchString(song.SHA256) || len(strings.TrimSpace(song.Title))<2 || len(song.Title)>150 || seen[song.ID] {
            return out,fmt.Errorf("invalid catalog entry %q",song.ID)
        }
        seen[song.ID]=true
    }
    return out,nil
}

func limitedGet(ctx context.Context,client *http.Client,url string,max int64)([]byte,error) {
    req,err:=http.NewRequestWithContext(ctx,"GET",url,nil);if err!=nil {return nil,err}
    req.Header.Set("User-Agent","Cazy-Lyre-Song-Sync/3.10")
    res,err:=client.Do(req);if err!=nil{return nil,err};defer res.Body.Close()
    if res.StatusCode!=200{return nil,fmt.Errorf("GitHub song library HTTP %d",res.StatusCode)}
    data,err:=io.ReadAll(io.LimitReader(res.Body,max+1));if err!=nil{return nil,err}
    if int64(len(data))>max{return nil,errors.New("download exceeds allowed size")}
    return data,nil
}

func downloadLyreSongs(ctx context.Context,client *http.Client,baseURL,installDir string)(songSyncReport,error) {
    var report songSyncReport
    index,err:=limitedGet(ctx,client,strings.TrimRight(baseURL,"/")+"/index.json",256*1024)
    if err!=nil{return report,err}
    catalog,err:=parseSongCatalog(index);if err!=nil{return report,err}
    if err=os.MkdirAll(installDir,0700);err!=nil{return report,err}
    for _,entry:=range catalog.Songs {
        target:=filepath.Join(installDir,entry.ID+"_"+entry.SHA256[:10]+".ahk")
        data,err:=os.ReadFile(target)
        if err==nil {
            sum:=sha256.Sum256(data)
            if hex.EncodeToString(sum[:])!=entry.SHA256 {return report,fmt.Errorf("local song %s has been edited; refusing to overwrite",entry.ID)}
            report.Unchanged++
            report.Imported=append(report.Imported,importedSong{target,entry.Title})
            continue
        }
        if !os.IsNotExist(err) {return report,err}
        data,err=limitedGet(ctx,client,strings.TrimRight(baseURL,"/")+"/songs/"+entry.ID+".ahk",1024*1024)
        if err!=nil{return report,fmt.Errorf("song %s: %w",entry.ID,err)}
        sum:=sha256.Sum256(data)
        if hex.EncodeToString(sum[:])!=entry.SHA256{return report,fmt.Errorf("song %s: SHA-256 mismatch",entry.ID)}
        // Only accept valid timed Cazy Lyre macros with the usual Mouse4/Mouse5/F12 controls.
        s:=string(data)
        _,rep,err:=applySongTempo(s,1.0)
        if err!=nil || rep.Events<2 || !strings.Contains(s,"XButton1::") || !strings.Contains(s,"XButton2::") || !strings.Contains(s,"F12::") {
            return report,fmt.Errorf("song %s: invalid or unsupported macro format",entry.ID)
        }
        temp,err:=os.CreateTemp(installDir,".song-*.tmp");if err!=nil{return report,err}
        name:=temp.Name()
        _,err=temp.Write(data)
        closeErr:=temp.Close()
        if err==nil {err=closeErr}
        if err!=nil {os.Remove(name);return report,err}
        // A versioned filename means existing user songs are never replaced.
        if err=os.Rename(name,target);err!=nil {os.Remove(name);return report,err}
        report.Downloaded++
        report.Imported=append(report.Imported,importedSong{target,entry.Title})
    }
    return report,nil
}
