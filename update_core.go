package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const lyreVersion = "3.10.0"

// Update feeds are plain HTTPS JSON. Each download must match the publisher's SHA-256.
type releaseManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Notes   string `json:"notes,omitempty"`
}
type releaseDescriptor struct {
	Version     string `json:"version"`
	Application string `json:"application"`
}

var versionPart = regexp.MustCompile(`^\d+$`)

func compareLyreVersions(a, b string) (int, error) {
	pa := strings.Split(strings.TrimPrefix(strings.TrimSpace(a), "v"), ".")
	pb := strings.Split(strings.TrimPrefix(strings.TrimSpace(b), "v"), ".")
	if len(pa) < 1 || len(pa) > 4 || len(pb) < 1 || len(pb) > 4 {
		return 0, fmt.Errorf("invalid version format")
	}
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		av, bv := uint64(0), uint64(0)
		if i < len(pa) {
			if !versionPart.MatchString(pa[i]) {
				return 0, errors.New("invalid version: " + a)
			}
			x, e := strconv.ParseUint(pa[i], 10, 32)
			if e != nil {
				return 0, e
			}
			av = x
		}
		if i < len(pb) {
			if !versionPart.MatchString(pb[i]) {
				return 0, errors.New("invalid version: " + b)
			}
			x, e := strconv.ParseUint(pb[i], 10, 32)
			if e != nil {
				return 0, e
			}
			bv = x
		}
		if av > bv {
			return 1, nil
		}
		if av < bv {
			return -1, nil
		}
	}
	return 0, nil
}

func verifyFileSHA256(filePath, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if len(expected) != 64 {
		return errors.New("update manifest must supply a 64-character SHA-256")
	}
	if _, e := hex.DecodeString(expected); e != nil {
		return errors.New("update manifest SHA-256 is not valid hex")
	}
	f, e := os.Open(filePath)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return e
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != expected {
		return errors.New("download hash mismatch; update rejected")
	}
	return nil
}

func safeReleaseEntry(s string) (string, error) {
	if s == "" || strings.ContainsAny(s, "\\:\x00") || strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("invalid update archive path: %q", s)
	}
	clean := path.Clean(s)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != strings.TrimSuffix(s, "/") {
		return "", fmt.Errorf("unsafe or noncanonical archive path: %q", s)
	}
	return clean, nil
}

// Stages a complete release out of the program directory. Only the standalone
// installer moves files after the old app process exits. Never unpack directly
// over user data or execute scripts from a downloaded ZIP.
func stageReleaseZip(zipPath, stageDir string, requiredVersion string) (releaseDescriptor, int, error) {
	var desc releaseDescriptor
	z, e := zip.OpenReader(zipPath)
	if e != nil {
		return desc, 0, e
	}
	defer z.Close()
	if len(z.File) == 0 || len(z.File) > 300 {
		return desc, 0, errors.New("invalid release archive file count")
	}
	entries := make([]string, len(z.File))
	commonRoot := ""
	wrapper := true
	for i, f := range z.File {
		n, e := safeReleaseEntry(f.Name)
		if e != nil {
			return desc, 0, e
		}
		entries[i] = n
		if f.Mode()&os.ModeSymlink != 0 {
			return desc, 0, errors.New("symbolic links not allowed in releases")
		}
		if !f.FileInfo().Mode().IsRegular() && !f.FileInfo().IsDir() {
			return desc, 0, errors.New("unsupported ZIP entry type")
		}
		bits := strings.Split(n, "/")
		if i == 0 {
			commonRoot = bits[0]
		}
		if bits[0] != commonRoot {
			wrapper = false
		}
		if !f.FileInfo().IsDir() && len(bits) < 2 {
			wrapper = false
		}
	}
	if e = os.MkdirAll(stageDir, 0700); e != nil {
		return desc, 0, e
	}
	count := 0
	total := int64(0)
	names := map[string]bool{}
	for i, f := range z.File {
		n := entries[i]
		if wrapper {
			n = strings.TrimPrefix(n, commonRoot+"/")
		}
		if n == "" || n == commonRoot && f.FileInfo().IsDir() {
			continue
		}
		if strings.HasPrefix(n, "/") || n == ".." || strings.HasPrefix(n, "../") {
			return desc, 0, errors.New("escaped archive root")
		}
		key := strings.ToLower(n)
		if names[key] {
			return desc, 0, fmt.Errorf("duplicate archive path: %s", n)
		}
		names[key] = true
		// Never overwrite local user collections, credentials, samples, or private environments.
		low := strings.ToLower(n)
		if low == "library.json" || strings.Contains(low, "api_key") || strings.HasPrefix(low, ".venv/") || strings.HasPrefix(low, "instrument_samples/") || strings.HasPrefix(low, "tempo exports/") {
			return desc, 0, errors.New("update ZIP contains protected user data: " + n)
		}
		out := filepath.Join(stageDir, filepath.FromSlash(n))
		rel, e := filepath.Rel(stageDir, out)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return desc, 0, errors.New("invalid ZIP output path")
		}
		if f.FileInfo().IsDir() {
			if e = os.MkdirAll(out, 0700); e != nil {
				return desc, 0, e
			}
			continue
		}
		if f.UncompressedSize64 > 150*1024*1024 {
			return desc, 0, errors.New("update contains oversized file")
		}
		if e = os.MkdirAll(filepath.Dir(out), 0700); e != nil {
			return desc, 0, e
		}
		src, e := f.Open()
		if e != nil {
			return desc, 0, e
		}
		dst, e := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			src.Close()
			return desc, 0, e
		}
		copied, copyErr := io.Copy(dst, io.LimitReader(src, 150*1024*1024+1))
		closeErr := dst.Close()
		src.Close()
		if copyErr != nil {
			return desc, 0, copyErr
		}
		if closeErr != nil {
			return desc, 0, closeErr
		}
		if copied > 150*1024*1024 {
			return desc, 0, errors.New("ZIP file exceeds size limit")
		}
		total += copied
		if total > 450*1024*1024 {
			return desc, 0, errors.New("ZIP exceeds size limit")
		}
		count++
	}
	info, e := os.ReadFile(filepath.Join(stageDir, "release.json"))
	if e != nil {
		return desc, 0, errors.New("release ZIP missing release.json")
	}
	if e = json.Unmarshal(info, &desc); e != nil {
		return desc, 0, fmt.Errorf("invalid release.json: %w", e)
	}
	if desc.Application != "Cazy Lyre" {
		return desc, 0, errors.New("update ZIP is not a Cazy Lyre release")
	}
	if _, e := os.Stat(filepath.Join(stageDir, "Cazy_Lyre.exe")); e != nil {
		return desc, 0, errors.New("release missing stable Cazy_Lyre.exe")
	}
	cmp, e := compareLyreVersions(desc.Version, lyreVersion)
	if e != nil {
		return desc, 0, e
	}
	if cmp <= 0 {
		return desc, 0, fmt.Errorf("release v%s is not newer than v%s", desc.Version, lyreVersion)
	}
	if requiredVersion != "" && desc.Version != requiredVersion {
		return desc, 0, fmt.Errorf("release version %s does not match advertised %s", desc.Version, requiredVersion)
	}
	return desc, count, nil
}
