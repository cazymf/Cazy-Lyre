package main

import (
	"strings"
	"testing"
)

func TestGithubRepo(t *testing.T) {
	for _, x := range []string{"cazymf/Cazy-Lyre", "owner/repo-2"} {
		if e := validGithubRepo(x); e != nil {
			t.Fatal(e)
		}
	}
	for _, x := range []string{"https://github.com/cazymf/Cazy-Lyre", "../evil", "owner/../private", "user/repo?x=1", "owner"} {
		if e := validGithubRepo(x); e == nil {
			t.Fatalf("accepted %q", x)
		}
	}
}
func TestSelectGithubLatest(t *testing.T) {
	url := "https://github.com/cazymf/Cazy-Lyre/releases/download/v3.10.0/Cazy_Lyre_Windows.zip"
	rs := []githubRelease{{Tag: "v3.8.0", Assets: []githubAsset{{Name: "Cazy_Lyre_Windows.zip", URL: strings.ReplaceAll(url, "v3.10.0", "v3.8.0"), Digest: "sha256:" + strings.Repeat("a", 64)}}}, {Tag: "v3.10.0", Assets: []githubAsset{{Name: "Cazy_Lyre_Windows.zip", URL: url, Digest: "sha256:" + strings.Repeat("b", 64)}}}, {Tag: "v9.0.0", Prerelease: true, Assets: []githubAsset{{Name: "Cazy_Lyre_Windows.zip", URL: url, Digest: "sha256:" + strings.Repeat("c", 64)}}}}
	m, _, e := chooseGithubLyre(rs)
	if e != nil || m.Version != "3.10.0" || m.SHA256 != strings.Repeat("b", 64) {
		t.Fatalf("got %#v %v", m, e)
	}
}
func TestGithubRejectOtherDomains(t *testing.T) {
	for _, u := range []string{"http://github.com/a", "https://github.com.evil.test/file", "https://evil.example/file", "file:///tmp/a.zip"} {
		if requireGithubDownloadURL(u) == nil {
			t.Fatalf("accepted %s", u)
		}
	}
}
