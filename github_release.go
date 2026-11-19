package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var repoPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,38}/[a-zA-Z0-9_.-]{1,100}$`)
var checksumPattern = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

type githubAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}
type githubRelease struct {
	Tag        string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Body       string        `json:"body"`
	Assets     []githubAsset `json:"assets"`
}

func validGithubRepo(repo string) error {
	repo = strings.TrimSpace(repo)
	if !repoPattern.MatchString(repo) || strings.Contains(repo, "..") || strings.HasSuffix(repo, ".") {
		return errors.New("enter a GitHub repository in owner/name format, e.g. cazymf/Cazy-Lyre")
	}
	return nil
}

func requireGithubDownloadURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u == nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return errors.New("GitHub update URLs must use HTTPS")
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "release-assets.githubusercontent.com" && host != "objects.githubusercontent.com" && host != "github-releases.githubusercontent.com" {
		return fmt.Errorf("GitHub download redirected to unexpected host: %s", host)
	}
	return nil
}

// The publisher uses conventional vMAJOR.MINOR.PATCH GitHub releases, with a
// Windows ZIP asset and a published SHA-256 digest or sibling .sha256 asset.
func chooseGithubLyre(releases []githubRelease) (releaseManifest, githubAsset, error) {
	var best releaseManifest
	var checksumAsset githubAsset
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease || !strings.HasPrefix(rel.Tag, "v") {
			continue
		}
		version := strings.TrimPrefix(rel.Tag, "v")
		if _, e := compareLyreVersions(version, "0"); e != nil {
			continue
		}
		if best.Version != "" {
			cmp, e := compareLyreVersions(version, best.Version)
			if e != nil || cmp <= 0 {
				continue
			}
		}
		var binary, checksum githubAsset
		for _, a := range rel.Assets {
			if a.Name == "Cazy_Lyre_Windows.zip" {
				binary = a
			}
			if a.Name == "Cazy_Lyre_Windows.zip.sha256" {
				checksum = a
			}
		}
		if binary.URL == "" {
			continue
		}
		if e := requireGithubDownloadURL(binary.URL); e != nil {
			continue
		}
		digest := strings.TrimPrefix(binary.Digest, "sha256:")
		if !checksumPattern.MatchString(digest) {
			digest = ""
		}
		if digest == "" && checksum.URL == "" {
			continue
		}
		if checksum.URL != "" {
			if e := requireGithubDownloadURL(checksum.URL); e != nil {
				continue
			}
		}
		best = releaseManifest{Version: version, URL: binary.URL, SHA256: strings.ToLower(digest), Notes: rel.Body}
		checksumAsset = checksum
	}
	return best, checksumAsset, nil
}

func fetchLatestGithubLyre(client *http.Client, repo string) (releaseManifest, error) {
	if e := validGithubRepo(repo); e != nil {
		return releaseManifest{}, e
	}
	uri := "https://api.github.com/repos/" + repo + "/releases?per_page=40"
	req, _ := http.NewRequest(http.MethodGet, uri, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Cazy-Lyre-GitHub-Updater/3.9")
	resp, e := client.Do(req)
	if e != nil {
		return releaseManifest{}, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return releaseManifest{}, fmt.Errorf("GitHub repo %s is not published yet (404). Run the GitHub setup once", repo)
		}
		if resp.StatusCode == 403 || resp.StatusCode == 429 {
			return releaseManifest{}, fmt.Errorf("GitHub API rate-limited update check (HTTP %d)", resp.StatusCode)
		}
		return releaseManifest{}, fmt.Errorf("GitHub releases request returned HTTP %d", resp.StatusCode)
	}
	var releases []githubRelease
	if e := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&releases); e != nil {
		return releaseManifest{}, e
	}
	found, sidecar, e := chooseGithubLyre(releases)
	if e != nil || found.Version == "" || found.SHA256 != "" {
		return found, e
	}
	req, _ = http.NewRequest(http.MethodGet, sidecar.URL, nil)
	req.Header.Set("User-Agent", "Cazy-Lyre-GitHub-Updater/3.9")
	checksumResponse, e := client.Do(req)
	if e != nil {
		return releaseManifest{}, e
	}
	defer checksumResponse.Body.Close()
	if checksumResponse.StatusCode != 200 {
		return releaseManifest{}, fmt.Errorf("GitHub checksum asset returned HTTP %d", checksumResponse.StatusCode)
	}
	contents, e := io.ReadAll(io.LimitReader(checksumResponse.Body, 4096))
	if e != nil {
		return releaseManifest{}, e
	}
	fields := strings.Fields(string(contents))
	if len(fields) == 0 || !checksumPattern.MatchString(fields[0]) {
		return releaseManifest{}, errors.New("GitHub release checksum is missing or invalid")
	}
	found.SHA256 = strings.ToLower(fields[0])
	return found, nil
}
