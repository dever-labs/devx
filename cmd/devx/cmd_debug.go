package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const launchFile = ".vscode/launch.json"

// runDebugConfig prints (or writes) VS Code attach configurations for every
// service that declares a debug block, so F5 attaches to what `devx up` started.
func runDebugConfig(args []string) error {
	fs := flag.NewFlagSet("debug-config", flag.ExitOnError)
	profile := fs.String("profile", "", "Profile to use")
	write := fs.Bool("write", false, "Merge into .vscode/launch.json instead of printing")
	_ = fs.Parse(args)

	_, _, prof, err := loadProfile(*profile)
	if err != nil {
		return err
	}

	var configs []map[string]any
	for _, name := range sortedServiceNames(prof) {
		svc := prof.Services[name]
		if svc.Debug == nil {
			continue
		}
		cfg, err := attachConfig(name, svc.Debug.Type, svc.Debug.Port)
		if err != nil {
			return fmt.Errorf("service %s: %w", name, err)
		}
		configs = append(configs, cfg)
	}
	if len(configs) == 0 {
		return fmt.Errorf("no service declares a debug block (debug: {port: 9229, type: node})")
	}

	if !*write {
		data, err := json.MarshalIndent(map[string]any{"version": "0.2.0", "configurations": configs}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	return mergeLaunch(configs)
}

func attachConfig(name, typ string, port int) (map[string]any, error) {
	label := "devx: " + name
	switch typ {
	case "node":
		return map[string]any{"type": "node", "request": "attach", "name": label, "address": "localhost", "port": port, "restart": true}, nil
	case "python":
		return map[string]any{"type": "debugpy", "request": "attach", "name": label, "connect": map[string]any{"host": "localhost", "port": port}}, nil
	case "go":
		return map[string]any{"type": "go", "request": "attach", "name": label, "mode": "remote", "host": "127.0.0.1", "port": port}, nil
	case "java":
		return map[string]any{"type": "java", "request": "attach", "name": label, "hostName": "localhost", "port": port}, nil
	case "":
		return nil, fmt.Errorf("debug.type is required (node, python, go, java)")
	default:
		return nil, fmt.Errorf("unsupported debug.type %q (node, python, go, java)", typ)
	}
}

// mergeLaunch replaces previously generated "devx: " entries and keeps the rest.
func mergeLaunch(configs []map[string]any) error {
	doc := map[string]any{"version": "0.2.0"}
	if data, err := os.ReadFile(launchFile); err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("%s is not plain JSON (comments?) — merge this output by hand:\n%v", launchFile, err)
		}
	}

	var merged []any
	if existing, ok := doc["configurations"].([]any); ok {
		for _, c := range existing {
			if m, ok := c.(map[string]any); ok {
				if n, _ := m["name"].(string); strings.HasPrefix(n, "devx: ") {
					continue
				}
			}
			merged = append(merged, c)
		}
	}
	for _, c := range configs {
		merged = append(merged, c)
	}
	doc["configurations"] = merged

	if err := os.MkdirAll(filepath.Dir(launchFile), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(launchFile, append(out, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("✓  wrote %d attach configuration(s) to %s\n", len(configs), launchFile)
	return nil
}
