package detect

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// nodeEngines is the subset of package.json needed for engines.node
// detection.
type nodeEngines struct {
	Engines struct {
		Node string `json:"node"`
	} `json:"engines"`
}

var semverPrefix = regexp.MustCompile(`^[^0-9]*`)

// NodeVersion looks for an explicit Node version pin in dir, checking (in
// priority order) .nvmrc, .node-version, then package.json's
// "engines.node" field. The returned version has any leading range
// operator or "v" prefix stripped (e.g. "^20.11.0" -> "20.11.0").
// Returns ("", false) if no pin is found in any of those sources.
func NodeVersion(dir string) (string, bool) {
	for _, name := range []string{".nvmrc", ".node-version"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if v := strings.TrimSpace(string(data)); v != "" {
			return cleanVersion(v), true
		}
	}

	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "", false
	}
	var pkg nodeEngines
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.Engines.Node == "" {
		return "", false
	}
	return cleanVersion(pkg.Engines.Node), true
}

func cleanVersion(v string) string {
	return semverPrefix.ReplaceAllString(strings.TrimSpace(v), "")
}

// NodeVersionManager reports the first installed Node version manager
// found on this machine, preferring volta and fnm (plain binaries found
// via PATH lookup) over nvm, which ships as a shell function rather than
// a binary and so is instead detected via NVM_DIR (or the conventional
// ~/.nvm) containing nvm.sh. Returns ("", false) if none are installed.
func NodeVersionManager() (string, bool) {
	for _, bin := range []string{"volta", "fnm"} {
		if _, err := exec.LookPath(bin); err == nil {
			return bin, true
		}
	}
	nvmDir := os.Getenv("NVM_DIR")
	if nvmDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			nvmDir = filepath.Join(home, ".nvm")
		}
	}
	if nvmDir != "" {
		if _, err := os.Stat(filepath.Join(nvmDir, "nvm.sh")); err == nil {
			return "nvm", true
		}
	}
	return "", false
}

// NodeInstallCommand returns the shell command that installs/activates
// version through manager (as returned by NodeVersionManager), instead of
// mutating the global toolchain via an OS package manager. Returns "" for
// an unrecognized manager name.
func NodeInstallCommand(manager, version string) string {
	switch manager {
	case "volta":
		return "volta install node@" + version
	case "fnm":
		return "fnm install " + version + " && fnm use " + version
	case "nvm":
		// Requires a shell that sources nvm.sh (e.g. an interactive
		// bash/zsh login shell); devx runs install commands as-is.
		return "nvm install " + version
	default:
		return ""
	}
}

// CorepackInstallCommand returns the Corepack invocation that activates
// the exact package manager version pinned in spec (package.json's
// "packageManager" field, e.g. "yarn@4.1.0"), when Corepack is available
// on PATH. Returns ("", false) if Corepack isn't installed — Corepack
// ships with Node >= 16.9, so this is effectively "is Node new enough".
func CorepackInstallCommand(spec string) (string, bool) {
	if _, err := exec.LookPath("corepack"); err != nil {
		return "", false
	}
	return "corepack prepare " + spec + " --activate", true
}
