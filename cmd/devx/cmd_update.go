package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// releasesAPI and releaseDL are vars (not consts) so tests can point them
// at a local httptest.Server instead of the real GitHub endpoints.
var (
	releasesAPI = "https://api.github.com/repos/dever-labs/devx/releases/latest"
	releaseDL   = "https://github.com/dever-labs/devx/releases/download"
)

func runUpdate(_ context.Context, args []string) error {
	check := len(args) > 0 && args[0] == "--check"

	fmt.Print("Checking for updates... ")
	latest, err := fetchLatestVersion()
	if err != nil {
		return fmt.Errorf("could not fetch release info: %w", err)
	}

	current := version
	if current == "dev" {
		fmt.Println()
		fmt.Println("Running dev build — skipping update.")
		return nil
	}

	if latest == current || latest == "v"+current {
		fmt.Printf("✓  already on latest (%s)\n", current)
		return nil
	}

	fmt.Printf("new version available: %s  (current: %s)\n", latest, current)

	if check {
		fmt.Println("Run  devx update  to install it.")
		return nil
	}

	return selfUpdate(latest)
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", releasesAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

func selfUpdate(tag string) error {
	assetName := binaryAssetName()
	url := fmt.Sprintf("%s/%s/%s", releaseDL, tag, assetName)

	fmt.Println("Fetching checksums...")
	wantSum, err := fetchChecksum(tag, assetName)
	if err != nil {
		return fmt.Errorf("could not fetch checksums.txt: %w", err)
	}

	// Write to a temp file next to the current binary.
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}

	fmt.Printf("Downloading %s...\n", assetName)
	tmp := exe + ".new"
	if err := downloadVerified(url, wantSum, tmp); err != nil {
		return err
	}

	// Atomically replace the running binary.
	backup := exe + ".old"
	os.Rename(exe, backup)
	if err := os.Rename(tmp, exe); err != nil {
		os.Rename(backup, exe) // restore on failure
		os.Remove(tmp)
		return fmt.Errorf("replacing binary failed: %w", err)
	}
	os.Remove(backup)

	fmt.Printf("✓  Updated to %s (checksum verified)\n", tag)
	return nil
}

// downloadVerified downloads url into destPath (mode 0755, as needed for a
// replacement binary), verifying its SHA-256 matches wantSum. destPath is
// removed and an error returned on any failure, including a checksum
// mismatch, so a bad download never gets applied.
func downloadVerified(url, wantSum, destPath string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("cannot write update (try with sudo?): %w", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(f, io.TeeReader(resp.Body, hasher)); err != nil {
		f.Close()
		os.Remove(destPath)
		return err
	}
	f.Close()

	gotSum := hex.EncodeToString(hasher.Sum(nil))
	if gotSum != wantSum {
		os.Remove(destPath)
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s (downloaded file was not applied)", filepath.Base(destPath), gotSum, wantSum)
	}

	return nil
}

// fetchChecksum downloads the release's checksums.txt (published by
// goreleaser alongside every binary) and returns the expected SHA-256 hex
// digest for assetName. checksums.txt lines look like:
//
//	<sha256sum>  <filename>
func fetchChecksum(tag, assetName string) (string, error) {
	url := fmt.Sprintf("%s/%s/checksums.txt", releaseDL, tag)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[1] == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}

	return "", fmt.Errorf("no checksum entry for %s", assetName)
}

func binaryAssetName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	name := fmt.Sprintf("devx-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// normaliseVersion strips a leading "v" for comparison.
func normaliseVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}
