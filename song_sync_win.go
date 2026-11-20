//go:build windows
package main

import (
    "context"
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"
)

var songSyncBusy bool

func beginSongSync(){
    if songSyncBusy || updater.busy {return}
    if err:=saveUpdaterRepo();err!=nil{
        msgBox("Song Sync",err.Error(),MB_OK|MB_ICONERROR)
        return
    }
    repo:=updater.repository
    // Only the user's configured GitHub repository is used; never download
    // from paths supplied by the catalog itself.
    base:="https://raw.githubusercontent.com/"+repo+"/main/song-library"
    songSyncBusy=true
    updater.state="Syncing songs from "+repo+"..."
    redraw()
    go func(){
        ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second)
        defer cancel()
        client:=&http.Client{Timeout:45*time.Second,CheckRedirect:func(r *http.Request,_ []*http.Request)error{
            if r.URL.Scheme!="https" || !strings.EqualFold(r.URL.Hostname(),"raw.githubusercontent.com"){
                return fmt.Errorf("unexpected Song Sync redirect: %s",r.URL.Hostname())
            }
            return nil
        }}
        home:=os.Getenv("LOCALAPPDATA")
        if home==""{home=os.TempDir()}
        songsDir:=filepath.Join(home,"Cazy Lyre","Synced Songs")
        report,err:=downloadLyreSongs(ctx,client,base,songsDir)
        postUpdateEvent(updateEvent{Kind:"songs",SongReport:report,Err:err})
    }()
}
