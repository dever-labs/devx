package plugins

import (
	"os"
	"path/filepath"
	"strings"
)

const Prefix = "devx-provider-"

func Discover() ([]string, error) {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil, nil
	}

	parts := filepath.SplitList(pathEnv)
	seen := map[string]bool{}
	var found []string
	for _, dir := range parts {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, Prefix) {
				continue
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			found = append(found, filepath.Join(dir, name))
		}
	}

	return found, nil
}
