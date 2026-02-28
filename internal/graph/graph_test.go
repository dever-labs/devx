package graph

import (
	"testing"

	"github.com/dever-labs/devx/internal/config"
)

func TestTopoSort(t *testing.T) {
	prof := &config.Profile{
		Services: map[string]config.Service{
			"api": {DependsOn: []string{"db"}},
		},
		Deps: map[string]config.Dep{
			"db": {Kind: "postgres"},
		},
	}

	g, err := Build(prof)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	order, err := TopoSort(g)
	if err != nil {
		t.Fatalf("toposort failed: %v", err)
	}

	if len(order) != 2 || order[0] != "db" {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestTopoSortCycle(t *testing.T) {
	prof := &config.Profile{
		Services: map[string]config.Service{
			"api": {DependsOn: []string{"web"}},
			"web": {DependsOn: []string{"api"}},
		},
	}

	g, err := Build(prof)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	if _, err := TopoSort(g); err == nil {
		t.Fatalf("expected cycle error")
	}
}
