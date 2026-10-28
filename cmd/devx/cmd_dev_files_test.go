package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDevcontainerJSON_NoFileReturnsError(t *testing.T) {
	chdirTemp(t)
	if _, _, err := loadDevcontainerJSON(); err == nil {
		t.Error("expected an error when no devcontainer.json exists")
	}
}

func TestLoadDevcontainerJSON_ReadsFromDevcontainerDir(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.Mkdir(filepath.Join(dir, ".devcontainer"), 0755); err != nil {
		t.Fatal(err)
	}
	content := `{
		// a comment
		"image": "mcr.microsoft.com/devcontainers/go:1"
	}`
	path := filepath.Join(dir, ".devcontainer", "devcontainer.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, gotPath, err := loadDevcontainerJSON()
	if err != nil {
		t.Fatalf("loadDevcontainerJSON failed: %v", err)
	}
	if gotPath != filepath.Join(".devcontainer", "devcontainer.json") {
		t.Errorf("gotPath = %q, want .devcontainer/devcontainer.json", gotPath)
	}
	if cfg.Image != "mcr.microsoft.com/devcontainers/go:1" {
		t.Errorf("cfg.Image = %q, want the go image", cfg.Image)
	}
}

func TestLoadDevcontainerJSON_FallsBackToRootFile(t *testing.T) {
	dir := chdirTemp(t)
	content := `{"image": "alpine"}`
	path := filepath.Join(dir, "devcontainer.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, gotPath, err := loadDevcontainerJSON()
	if err != nil {
		t.Fatalf("loadDevcontainerJSON failed: %v", err)
	}
	if gotPath != "devcontainer.json" {
		t.Errorf("gotPath = %q, want devcontainer.json", gotPath)
	}
	if cfg.Image != "alpine" {
		t.Errorf("cfg.Image = %q, want alpine", cfg.Image)
	}
}

func TestLoadDevcontainerJSON_InvalidJSONErrors(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "devcontainer.json"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadDevcontainerJSON(); err == nil {
		t.Error("expected an error for invalid JSON")
	}
}

func TestFindBaseImages_NoImagesDirReturnsError(t *testing.T) {
	chdirTemp(t)
	if _, err := findBaseImages(); err == nil {
		t.Error("expected an error when there's no images/ directory")
	}
}

func TestFindBaseImages_FindsDirsWithDevcontainerJSON(t *testing.T) {
	dir := chdirTemp(t)
	goDev := filepath.Join(dir, "images", "go-dev")
	if err := os.MkdirAll(goDev, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goDev, "devcontainer.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	// A directory without devcontainer.json should be skipped.
	noJSON := filepath.Join(dir, "images", "not-an-image")
	if err := os.MkdirAll(noJSON, 0755); err != nil {
		t.Fatal(err)
	}

	images, err := findBaseImages()
	if err != nil {
		t.Fatalf("findBaseImages failed: %v", err)
	}
	if len(images) != 1 || images[0] != "go-dev" {
		t.Errorf("findBaseImages() = %v, want [go-dev]", images)
	}
}
