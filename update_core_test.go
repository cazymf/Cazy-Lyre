package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareLyreVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"3.8", "3.7", 1}, {"3.8.0", "3.8", 0}, {"3.9", "3.10", -1}, {"v4.0.0", "3.8.0", 1}} {
		got, e := compareLyreVersions(tc.a, tc.b)
		if e != nil || got != tc.want {
			t.Fatalf("%v => got %d %v", tc, got, e)
		}
	}
}
func TestUnsafeZipEntry(t *testing.T) {
	for _, s := range []string{"../bad.txt", "C:/Windows/x", "/etc/passwd", "folder/../evil.txt", `dir\\file.exe`} {
		if _, e := safeReleaseEntry(s); e == nil {
			t.Fatalf("unexpected acceptance: %q", s)
		}
	}
}
func TestStageRelease(t *testing.T) {
	tmp := t.TempDir()
	f, e := os.Create(filepath.Join(tmp, "update.zip"))
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	for n, content := range map[string]string{"Cazy_Lyre_v3_10/release.json": `{"version":"3.11.1","application":"Cazy Lyre"}`, "Cazy_Lyre_v3_10/Cazy_Lyre.exe": "MZdummy", "Cazy_Lyre_v3_10/README.txt": "Works"} {
		w, e := z.Create(n)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(content)); e != nil {
			t.Fatal(e)
		}
	}
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	f.Close()
	desc, n, e := stageReleaseZip(f.Name(), filepath.Join(tmp, "stage"), "3.11.1")
	if e != nil {
		t.Fatal(e)
	}
	if desc.Version != "3.11.1" || n != 3 {
		t.Fatalf("unexpected result %#v %d", desc, n)
	}
	if _, e = os.Stat(filepath.Join(tmp, "stage", "Cazy_Lyre.exe")); e != nil {
		t.Fatal(e)
	}
}
func TestStageRejectOldRelease(t *testing.T) {
	tmp := t.TempDir()
	f, _ := os.Create(filepath.Join(tmp, "old.zip"))
	z := zip.NewWriter(f)
	for n, c := range map[string]string{"release.json": `{"version":"3.7.0","application":"Cazy Lyre"}`, "Cazy_Lyre.exe": "MZdummy"} {
		w, _ := z.Create(n)
		w.Write([]byte(c))
	}
	z.Close()
	f.Close()
	_, _, e := stageReleaseZip(f.Name(), filepath.Join(tmp, "stage"), "")
	if e == nil || !strings.Contains(e.Error(), "not newer") {
		t.Fatalf("wanted old-version rejection; got %v", e)
	}
}

func TestReleaseJSONMatchesBinaryVersion(t *testing.T) {
	raw, err := os.ReadFile("release.json")
	if err != nil {
		t.Fatal(err)
	}
	var d releaseDescriptor
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d.Application != "Cazy Lyre" || d.Version != lyreVersion {
		t.Fatalf("release.json version %q differs from compiled app %q", d.Version, lyreVersion)
	}
}
