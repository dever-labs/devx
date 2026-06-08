package main

import (
	"context"
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

const (
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

	fmt.Printf("Downloading %s...\n", assetName)

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
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

	tmp := exe + ".new"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("cannot write update (try with sudo?): %w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()

	// Atomically replace the running binary.
	backup := exe + ".old"
	os.Rename(exe, backup)
	if err := os.Rename(tmp, exe); err != nil {
		os.Rename(backup, exe) // restore on failure
		os.Remove(tmp)
		return fmt.Errorf("replacing binary failed: %w", err)
	}
	os.Remove(backup)

	fmt.Printf("✓  Updated to %s\n", tag)
	return nil
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
