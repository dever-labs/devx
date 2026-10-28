package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunRender_ReturnsDocumentedError(t *testing.T) {
	if err := runRender(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "devx render") {
		t.Errorf("runRender() = %v, want an error mentioning 'devx render'", err)
	}
}

func TestRunLock_ReturnsDocumentedError(t *testing.T) {
	if err := runLock(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "devx lock") {
		t.Errorf("runLock() = %v, want an error mentioning 'devx lock'", err)
	}
}

func TestRunProviders_ReturnsDocumentedError(t *testing.T) {
	if err := runProviders(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "devx providers") {
		t.Errorf("runProviders() = %v, want an error mentioning 'devx providers'", err)
	}
}

func TestRunExport_ReturnsDocumentedError(t *testing.T) {
	if err := runExport(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "devx export") {
		t.Errorf("runExport() = %v, want an error mentioning 'devx export'", err)
	}
}
