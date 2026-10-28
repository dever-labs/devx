package main

import (
	"context"
	"errors"
)

// runRender/runLock/runProviders/runExport are stubs: these commands
// were entirely built on the old Profile/Service/Dep + Docker Compose/
// Kubernetes rendering model, which this version of devx no longer
// generates (see docs/design/task-executor-refactor.md — the whole
// point of the task model is that run: is plain shell text devx
// executes directly, not a declarative shape devx renders to compose/k8s
// YAML). There is no new-world equivalent to add here.

func runRender(ctx context.Context, args []string) error {
	return errors.New("devx render is not supported in the task-based devx.yaml model — run: is executed directly, not rendered to compose/k8s YAML; see docs/design/task-executor-refactor.md")
}

func runLock(ctx context.Context, args []string) error {
	return errors.New("devx lock is not supported in the task-based devx.yaml model — there are no provider-managed dep images to pin; see docs/design/task-executor-refactor.md")
}

func runProviders(ctx context.Context, args []string) error {
	return errors.New("devx providers is not supported in the task-based devx.yaml model — dep provider plugins have been removed; see docs/design/task-executor-refactor.md")
}

func runExport(ctx context.Context, args []string) error {
	return errors.New("devx export is not supported in the task-based devx.yaml model — there's no Profile/Service shape left to generate compose/k8s/helm/terraform from; see docs/design/task-executor-refactor.md")
}
