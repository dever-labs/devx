package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/dever-labs/devx/internal/task"
)

// runTasks implements `devx tasks` / `devx tasks --json`, dumping the
// full resolved task list — the structured data backing --help.
func runTasks(args []string) error {
	fs := flag.NewFlagSet("tasks", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "Emit the task list as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	m, err := loadValidManifest()
	if err != nil {
		return err
	}
	sched, err := newScheduler(m, nil, task.Options{})
	if err != nil {
		return err
	}
	resolved, err := sched.ResolveAll()
	if err != nil {
		return err
	}

	names := make([]string, 0, len(resolved))
	for n := range resolved {
		names = append(names, n)
	}
	sort.Strings(names)

	if *jsonOut {
		type jsonTask struct {
			Name          string            `json:"name"`
			Desc          string            `json:"desc,omitempty"`
			Group         string            `json:"group,omitempty"`
			Deps          []string          `json:"deps,omitempty"`
			Run           string            `json:"run"`
			Ready         *jsonReady        `json:"ready,omitempty"`
			Detached      bool              `json:"detached"`
			Env           map[string]string `json:"env,omitempty"`
			OS            []string          `json:"os,omitempty"`
			Interactive   bool              `json:"interactive,omitempty"`
			Sources       []string          `json:"sources,omitempty"`
			Generates     []string          `json:"generates,omitempty"`
			Shell         string            `json:"shell,omitempty"`
			Timeout       string            `json:"timeout,omitempty"`
			Restart       string            `json:"restart,omitempty"`
			StopTimeout   string            `json:"stopTimeout,omitempty"`
			Dir           string            `json:"dir"`
			Requires      []string          `json:"requires,omitempty"`
			ContainerName string            `json:"containerName,omitempty"`
			Scope         string            `json:"scope,omitempty"`
			Port          int               `json:"port,omitempty"`
		}
		out := make([]jsonTask, 0, len(names))
		for _, n := range names {
			et := resolved[n]
			jt := jsonTask{
				Name: n, Desc: et.Desc, Group: et.Group, Deps: et.Deps,
				Run: et.ResolvedRun, Detached: et.Detached,
				Env: task.RedactedEnv(et.ResolvedEnv), OS: et.OS,
				Interactive: et.Interactive, Sources: et.Sources, Generates: et.Generates,
				Shell: et.Shell, Timeout: et.Timeout, Restart: et.Restart,
				StopTimeout: et.StopTimeout, Dir: et.ResolvedDir, Requires: et.Requires,
				ContainerName: et.ContainerName, Scope: et.Scope, Port: et.Port,
			}
			if et.ResolvedReady != nil {
				jt.Ready = &jsonReady{HTTP: et.ResolvedReady.HTTP, TCP: et.ResolvedReady.TCP, Cmd: et.ResolvedReady.Cmd}
			}
			out = append(out, jt)
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	maxLen := 0
	for _, n := range names {
		if len(n) > maxLen {
			maxLen = len(n)
		}
	}
	for _, n := range names {
		et := resolved[n]
		desc := et.Desc
		if desc == "" {
			desc = et.ResolvedRun
		}
		shape := "one-shot"
		if et.Detached {
			shape = "detached"
		}
		fmt.Fprintf(os.Stdout, "  %-*s  [%s]  %s\n", maxLen, n, shape, desc)
	}
	return nil
}

type jsonReady struct {
	HTTP string `json:"http,omitempty"`
	TCP  string `json:"tcp,omitempty"`
	Cmd  string `json:"cmd,omitempty"`
}
