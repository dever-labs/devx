package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBinaryAssetName(t *testing.T) {
	got := binaryAssetName()
	want := "devx-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if got != want {
		t.Errorf("binaryAssetName() = %q, want %q", got, want)
	}
}

func TestNormaliseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := normaliseVersion(tc.in); got != tc.want {
			t.Errorf("normaliseVersion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func withFakeReleasesAPI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	orig := releasesAPI
	releasesAPI = srv.URL
	t.Cleanup(func() { releasesAPI = orig })
}

func TestFetchLatestVersion_Success(t *testing.T) {
	withFakeReleasesAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	})
	got, err := fetchLatestVersion()
	if err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if got != "v9.9.9" {
		t.Errorf("fetchLatestVersion() = %q, want v9.9.9", got)
	}
}

func TestFetchLatestVersion_MalformedJSONErrors(t *testing.T) {
	withFakeReleasesAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	if _, err := fetchLatestVersion(); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestFetchLatestVersion_UnreachableServerErrors(t *testing.T) {
	orig := releasesAPI
	releasesAPI = "http://127.0.0.1:1" // nothing listens here
	defer func() { releasesAPI = orig }()

	if _, err := fetchLatestVersion(); err == nil {
		t.Fatal("expected an error for an unreachable releases API")
	}
}

func TestRunUpdate_DevBuildSkipsUpdate(t *testing.T) {
	withFakeReleasesAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	})
	origVersion := version
	version = "dev"
	defer func() { version = origVersion }()

	if err := runUpdate(context.Background(), nil); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
}

func TestRunUpdate_AlreadyOnLatest(t *testing.T) {
	withFakeReleasesAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	})
	origVersion := version
	version = "1.2.3"
	defer func() { version = origVersion }()

	if err := runUpdate(context.Background(), nil); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
}

func TestRunUpdate_CheckFlagDoesNotUpdate(t *testing.T) {
	withFakeReleasesAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	})
	origVersion := version
	version = "1.2.3"
	defer func() { version = origVersion }()

	if err := runUpdate(context.Background(), []string{"--check"}); err != nil {
		t.Fatalf("runUpdate(--check): %v", err)
	}
}

func TestRunUpdate_FetchErrorPropagates(t *testing.T) {
	orig := releasesAPI
	releasesAPI = "http://127.0.0.1:1"
	defer func() { releasesAPI = orig }()

	origVersion := version
	version = "1.2.3"
	defer func() { version = origVersion }()

	if err := runUpdate(context.Background(), nil); err == nil {
		t.Fatal("expected an error when fetching the release info fails")
	}
}

func withFakeReleaseDL(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	orig := releaseDL
	releaseDL = srv.URL
	t.Cleanup(func() { releaseDL = orig })
}

func TestFetchChecksum_Success(t *testing.T) {
	withFakeReleaseDL(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("deadbeef  devx-linux-amd64\ncafef00d  devx-darwin-arm64\n"))
	})
	got, err := fetchChecksum("v9.9.9", "devx-linux-amd64")
	if err != nil {
		t.Fatalf("fetchChecksum: %v", err)
	}
	if got != "deadbeef" {
		t.Errorf("fetchChecksum() = %q, want deadbeef", got)
	}
}

func TestFetchChecksum_NoEntryErrors(t *testing.T) {
	withFakeReleaseDL(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("cafef00d  devx-darwin-arm64\n"))
	})
	if _, err := fetchChecksum("v9.9.9", "devx-linux-amd64"); err == nil {
		t.Fatal("expected an error when the asset has no checksum entry")
	}
}

func TestFetchChecksum_HTTPErrorPropagates(t *testing.T) {
	withFakeReleaseDL(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := fetchChecksum("v9.9.9", "devx-linux-amd64"); err == nil {
		t.Fatal("expected an error on HTTP 404")
	}
}

func TestDownloadVerified_Success(t *testing.T) {
	content := []byte("fake devx binary contents")
	sum := sha256.Sum256(content)
	wantSum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "devx.new")
	if err := downloadVerified(srv.URL, wantSum, dest); err != nil {
		t.Fatalf("downloadVerified: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("downloaded content = %q, want %q", got, content)
	}
}

func TestDownloadVerified_ChecksumMismatchRemovesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tampered or corrupted contents"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "devx.new")
	err := downloadVerified(srv.URL, "0000000000000000000000000000000000000000000000000000000000000000", dest)
	if err == nil {
		t.Fatal("expected a checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %q, want it to mention a checksum mismatch", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("expected the downloaded file to be removed after a checksum mismatch")
	}
}

func TestDownloadVerified_HTTPErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "devx.new")
	if err := downloadVerified(srv.URL, "anything", dest); err == nil {
		t.Fatal("expected an error on HTTP 500")
	}
}
