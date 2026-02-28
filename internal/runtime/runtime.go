package runtime

import (
	"context"
	"errors"
	"io"
)

type UpOptions struct {
	Build bool
	Pull  bool
}

type LogsOptions struct {
	Service string
	Follow  bool
	Since   string
	JSON    bool
}

type ServiceStatus struct {
	Name       string
	State      string
	Health     string
	Ports      string
	Publishers []Publisher
}

// Publisher represents an actual host-port binding as reported by the container runtime.
type Publisher struct {
	URL           string
	TargetPort    int
	PublishedPort int
	Protocol      string
}

type Runtime interface {
	Name() string
	Detect(ctx context.Context) (bool, error)
	Up(ctx context.Context, composePath string, projectName string, opts UpOptions) error
	Down(ctx context.Context, composePath string, projectName string, removeVolumes bool) error
	Logs(ctx context.Context, composePath string, projectName string, opts LogsOptions) (io.ReadCloser, error)
	Exec(ctx context.Context, composePath string, projectName string, service string, cmd []string) (int, error)
	Status(ctx context.Context, composePath string, projectName string) ([]ServiceStatus, error)
}

type DigestResolver interface {
	ResolveImageDigest(ctx context.Context, image string) (string, error)
}

type RuntimeInfo struct {
	Name      string
	Available bool
	Version   string
	Compose   bool
	Details   string
}

var ErrNoRuntime = errors.New("no container runtime detected")
