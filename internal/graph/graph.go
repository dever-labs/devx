package graph

import (
	"container/heap"
	"fmt"

	"github.com/dever-labs/devx/internal/config"
)

type Node struct {
	Name      string
	DependsOn []string
}

type Graph struct {
	Nodes map[string]Node
}

// Build constructs a dependency graph directly from a Manifest's resolved,
// namespaced Tasks map (see config.Load) — the single scheduler for every
// task, including across includes: boundaries.
func Build(tasks map[string]config.Task) (*Graph, error) {
	nodes := make(map[string]Node, len(tasks))
	for name, t := range tasks {
		nodes[name] = Node{
			Name:      name,
			DependsOn: append([]string{}, t.Deps...),
		}
	}
	return &Graph{Nodes: nodes}, nil
}

// stringHeap is a min-heap of strings for deterministic topological ordering.
type stringHeap []string

func (h stringHeap) Len() int           { return len(h) }
func (h stringHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h stringHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *stringHeap) Push(x any)        { *h = append(*h, x.(string)) }
func (h *stringHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// TopoSort returns a deterministic topological order of g's nodes, or an
// error on an unknown dependency or a cycle.
func TopoSort(g *Graph) ([]string, error) {
	if g == nil {
		return nil, nil
	}

	indegree := map[string]int{}
	adj := map[string][]string{}

	for name := range g.Nodes {
		indegree[name] = 0
	}
	for name, node := range g.Nodes {
		for _, dep := range node.DependsOn {
			adj[dep] = append(adj[dep], name)
		}
	}

	for name, node := range g.Nodes {
		for _, dep := range node.DependsOn {
			if _, ok := g.Nodes[dep]; !ok {
				return nil, fmt.Errorf("unknown dependency '%s' for '%s'", dep, name)
			}
			indegree[name]++
		}
	}

	h := &stringHeap{}
	heap.Init(h)
	for name, count := range indegree {
		if count == 0 {
			heap.Push(h, name)
		}
	}

	var order []string
	for h.Len() > 0 {
		current := heap.Pop(h).(string)
		order = append(order, current)

		for _, next := range adj[current] {
			indegree[next]--
			if indegree[next] == 0 {
				heap.Push(h, next)
			}
		}
	}

	if len(order) != len(g.Nodes) {
		return nil, fmt.Errorf("dependency cycle detected")
	}

	return order, nil
}

// Closure returns the transitive set of names reachable by following
// DependsOn from each of the given roots (inclusive of the roots
// themselves) — used for partial `up`/`run <name...>` invocations.
func Closure(g *Graph, roots []string) (map[string]bool, error) {
	seen := map[string]bool{}
	var visit func(name string) error
	visit = func(name string) error {
		if seen[name] {
			return nil
		}
		node, ok := g.Nodes[name]
		if !ok {
			return fmt.Errorf("unknown task '%s'", name)
		}
		seen[name] = true
		for _, dep := range node.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		return nil
	}
	for _, r := range roots {
		if err := visit(r); err != nil {
			return nil, err
		}
	}
	return seen, nil
}
