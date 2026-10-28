package graph

import (
	"testing"

	"github.com/dever-labs/devx/internal/config"
)

func TestBuildAndTopoSort(t *testing.T) {
	tasks := map[string]config.Task{
		"db":      {Name: "db", Run: "docker run -d postgres"},
		"migrate": {Name: "migrate", Run: "./migrate.sh", Deps: []string{"db"}},
		"api":     {Name: "api", Run: "./api", Deps: []string{"migrate"}},
	}
	g, err := Build(tasks)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	order, err := TopoSort(g)
	if err != nil {
		t.Fatalf("toposort failed: %v", err)
	}
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if pos["db"] > pos["migrate"] || pos["migrate"] > pos["api"] {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestTopoSort_DetectsCycle(t *testing.T) {
	tasks := map[string]config.Task{
		"a": {Name: "a", Run: "echo a", Deps: []string{"b"}},
		"b": {Name: "b", Run: "echo b", Deps: []string{"a"}},
	}
	g, _ := Build(tasks)
	if _, err := TopoSort(g); err == nil {
		t.Fatalf("expected cycle error")
	}
}

func TestTopoSort_UnknownDependency(t *testing.T) {
	tasks := map[string]config.Task{
		"a": {Name: "a", Run: "echo a", Deps: []string{"missing"}},
	}
	g, _ := Build(tasks)
	if _, err := TopoSort(g); err == nil {
		t.Fatalf("expected unknown dependency error")
	}
}

func TestClosure(t *testing.T) {
	tasks := map[string]config.Task{
		"db":      {Name: "db", Run: "docker run -d postgres"},
		"cache":   {Name: "cache", Run: "docker run -d redis"},
		"migrate": {Name: "migrate", Run: "./migrate.sh", Deps: []string{"db"}},
		"api":     {Name: "api", Run: "./api", Deps: []string{"migrate"}},
	}
	g, _ := Build(tasks)
	set, err := Closure(g, []string{"api"})
	if err != nil {
		t.Fatalf("closure failed: %v", err)
	}
	for _, want := range []string{"api", "migrate", "db"} {
		if !set[want] {
			t.Fatalf("expected %q in closure, got %v", want, set)
		}
	}
	if set["cache"] {
		t.Fatalf("unrelated task 'cache' should not be in api's closure")
	}
}
