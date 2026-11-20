package main
import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"
)
func TestSongSyncImportsPreservesAndSkips(t *testing.T) {
    content:=[]byte("#Requires AutoHotkey v2.0\nnotes := [\n [0, \"QW\"],\n [200, \"C\"]\n]\nXButton1::Start()\nXButton2::Stop()\nF12::ExitApp()\n")
    digest:=sha256.Sum256(content); sha:=hex.EncodeToString(digest[:])
    requests:=0
    srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        requests++
        switch r.URL.Path {
        case "/index.json":fmt.Fprintf(w,`{"format":1,"songs":[{"id":"test-song","title":"Test Song","sha256":"%s"}]}`,sha)
        case "/songs/test-song.ahk":w.Write(content)
        default:http.NotFound(w,r)
        }
    }))
    defer srv.Close()
    dir:=t.TempDir()
    first,err:=downloadLyreSongs(context.Background(),srv.Client(),srv.URL,dir)
    if err!=nil||first.Downloaded!=1||len(first.Imported)!=1{t.Fatalf("first sync: %+v %v",first,err)}
    second,err:=downloadLyreSongs(context.Background(),srv.Client(),srv.URL,dir)
    if err!=nil||second.Downloaded!=0||second.Unchanged!=1||requests!=3 {t.Fatalf("second sync: %+v %v requests=%d",second,err,requests)}
    if err:=os.WriteFile(first.Imported[0].Path,[]byte("personal modification"),0600);err!=nil{t.Fatal(err)}
    _,err=downloadLyreSongs(context.Background(),srv.Client(),srv.URL,dir)
    if err==nil||!strings.Contains(err.Error(),"refusing to overwrite"){t.Fatalf("edited file should survive: %v",err)}
    b,_:=os.ReadFile(filepath.Join(dir,"test-song_"+sha[:10]+".ahk"))
    if string(b)!="personal modification"{t.Fatal("local edit overwritten")}
}
func TestSongCatalogRejectsTraversal(t *testing.T){
    _,err:=parseSongCatalog([]byte(`{"format":1,"songs":[{"id":"../bad","title":"Bad Thing","sha256":"`+strings.Repeat("a",64)+`"}]}`))
    if err==nil {t.Fatal("traversal id accepted")}
}
