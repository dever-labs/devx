package task

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// osMatches reports whether the current GOOS satisfies an os: filter
// (empty list means no restriction). "macos" is accepted as an alias for
// "darwin" since the manifest examples use the human-friendly name.
func osMatches(list []string) bool {
	if len(list) == 0 {
		return true
	}
	for _, want := range list {
		if want == runtime.GOOS {
			return true
		}
		if want == "macos" && runtime.GOOS == "darwin" {
			return true
		}
	}
	return false
}

// upToDate reports whether a one-shot task's generates: outputs are all
// newer than every sources: input — Make's own core "skip if up to date"
// behavior, applied to globs. Only meaningful when both fields are set;
// returns false (always re-run) otherwise, including when either glob
// set matches nothing (nothing to skip against yet).
func upToDate(et *EffectiveTask) bool {
	if len(et.Sources) == 0 || len(et.Generates) == 0 {
		return false
	}
	newestSource, ok := mtimeExtreme(et.ResolvedDir, et.Sources, true)
	if !ok {
		return false
	}
	oldestGenerated, ok := mtimeExtreme(et.ResolvedDir, et.Generates, false)
	if !ok {
		return false
	}
	return oldestGenerated.After(newestSource)
}

// mtimeExtreme returns the newest (newest=true) or oldest (newest=false)
// mtime among every file matched by globs (resolved relative to dir).
// ok is false if no file matched any glob.
func mtimeExtreme(dir string, globs []string, newest bool) (time.Time, bool) {
	var extreme time.Time
	found := false
	for _, pattern := range globs {
		full := pattern
		if !filepath.IsAbs(full) {
			full = filepath.Join(dir, pattern)
		}
		matches, err := filepath.Glob(full)
		if err != nil {
			continue
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				continue
			}
			mt := info.ModTime()
			if !found {
				extreme = mt
				found = true
				continue
			}
			if newest && mt.After(extreme) {
				extreme = mt
			}
			if !newest && mt.Before(extreme) {
				extreme = mt
			}
		}
	}
	return extreme, found
}
