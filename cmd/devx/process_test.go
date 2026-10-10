package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/config"
)

func TestHostPort(t *testing.T) {
	for in, want := range map[string]string{"3000": "3000", "8080:80": "8080", "127.0.0.1:8080:80": "8080"} {
		if got := hostPort(in); got != want {
			t.Errorf("hostPort(%q)=%q want %q", in, got, want)
		}
	}
}

func TestHostVals(t *testing.T) {
	got := hostVals(map[string]string{"host": "db", "port": "5432", "POSTGRES_DB": "app"}, []string{"15432:5432"})
	if got["host"] != "localhost" || got["port"] != "15432" || got["POSTGRES_DB"] != "app" {
		t.Fatalf("unexpected vals %v", got)
	}
}

func TestProbeFor(t *testing.T) {
	p := probeFor("web", config.Service{Kind: config.KindProcess, Ports: []string{"3000"}})
	if p.TCP != "localhost:3000" || p.HTTP != "" {
		t.Fatalf("process with port should default to tcp probe, got %+v", p)
	}
	p = probeFor("web", config.Service{Kind: config.KindProcess, Ports: []string{"3000"}, Ready: &config.Ready{HTTP: "http://localhost:3000/h", Timeout: "30s"}})
	if p.HTTP == "" || p.TCP != "" || p.Timeout != 30*time.Second {
		t.Fatalf("explicit ready must win, got %+v", p)
	}
	p = probeFor("api", config.Service{Image: "x", Health: &config.Health{HttpGet: "http://localhost/h"}})
	if p.HTTP != "http://localhost/h" {
		t.Fatalf("health.httpGet fallback failed: %+v", p)
	}
	if !probeFor("api", config.Service{Image: "x", Ports: []string{"80"}}).Empty() {
		t.Fatal("container with ports and no probe must not get a default probe")
	}
}

func TestSplitProcessPhases(t *testing.T) {
	prof := &config.Profile{
		Services: map[string]config.Service{
			"gen":    {Kind: config.KindProcess, Run: "x"},
			"web":    {Kind: config.KindProcess, Run: "x", DependsOn: []string{"gen", "db"}},
			"worker": {Kind: config.KindProcess, Run: "x", DependsOn: []string{"web"}},
			"api":    {Image: "x", DependsOn: []string{"gen"}},
		},
		Deps: map[string]config.Dep{"db": {Image: "postgres"}},
	}
	before, after, err := splitProcessPhases(prof)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, []string{"gen"}) || !reflect.DeepEqual(after, []string{"web", "worker"}) {
		t.Fatalf("before=%v after=%v", before, after)
	}
}
