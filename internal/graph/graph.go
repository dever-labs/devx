package graph

import (
	"container/heap"
	"fmt"

	"github.com/dever-labs/devx/internal/config"
)

type Node struct {
	Name      string
	Kind      string
	DependsOn []string
}

type Graph struct {
	Nodes map[string]Node
}

func Build(profile *config.Profile) (*Graph, error) {
	nodes := make(map[string]Node)

	if profile == nil {
		return &Graph{Nodes: nodes}, nil
	}

	for name, svc := range profile.Services {
		nodes[name] = Node{
			Name:      name,
			Kind:      "service",
			DependsOn: append([]string{}, svc.DependsOn...),
		}
	}

	for name := range profile.Deps {
		if _, ok := nodes[name]; ok {
			return nil, fmt.Errorf("name '%s' is used by both service and dep", name)
		}
		nodes[name] = Node{
			Name:      name,
			Kind:      "dep",
			DependsOn: nil,
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

func TopoSort(g *Graph) ([]string, error) {
	if g == nil {
		return nil, nil
	}

	indegree := map[string]int{}
	adj := map[string][]string{}

	for name, node := range g.Nodes {
		indegree[name] = 0
		for _, dep := range node.DependsOn {
			adj[dep] = append(adj[dep], name)
		}
	}

	for name, node := range g.Nodes {
		for _, dep := range node.DependsOn {
			if _, ok := indegree[dep]; !ok {
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
