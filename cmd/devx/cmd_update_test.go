package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
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
