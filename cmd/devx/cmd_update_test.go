package main

import (
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
