package compose

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/lock"
	"github.com/dever-labs/devx/internal/util"
	"gopkg.in/yaml.v3"
)

type File struct {
	Services map[string]Service `yaml:"services"`
	Networks map[string]Network `yaml:"networks,omitempty"`
	Volumes  map[string]Volume  `yaml:"volumes,omitempty"`
}

type Network struct{}

type Volume struct{}

type Service struct {
	Image       string            `yaml:"image,omitempty"`
	Build       *Build            `yaml:"build,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Command     []string          `yaml:"command,omitempty"`
	WorkingDir  string            `yaml:"working_dir,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	DependsOn   []string          `yaml:"depends_on,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Healthcheck *Healthcheck      `yaml:"healthcheck,omitempty"`
	Networks    []string          `yaml:"networks,omitempty"`
	Privileged  bool              `yaml:"privileged,omitempty"`
	ExtraHosts  []string          `yaml:"extra_hosts,omitempty"`
}

type Build struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
}

type Healthcheck struct {
	Test     []string `yaml:"test"`
	Interval string   `yaml:"interval,omitempty"`
	Retries  int      `yaml:"retries,omitempty"`
}

type RewriteOptions struct {
	RegistryPrefix string
	Lockfile       *lock.Lockfile
	// DepFragments holds optional provider-contributed configuration keyed by
	// dep name. If a fragment is present for a dep, its fields are merged into
	// the rendered compose service (e.g. a healthcheck).
	DepFragments map[string]*DepFragment
}

// DepFragment is the optional provider-contributed configuration for a dep
// service. Fields present in the fragment are merged into the base service
// rendered from the dep's own devx.yaml config.
type DepFragment struct {
	Healthcheck *Healthcheck `json:"healthcheck,omitempty"`
}

func Render(manifest *config.Manifest, profileName string, profile *config.Profile, rewrite RewriteOptions, enableTelemetry bool) (string, error) {
	if manifest == nil || profile == nil {
		return "", fmt.Errorf("manifest and profile are required")
	}

	file := File{
		Services: map[string]Service{},
		Networks: map[string]Network{"devx_default": {}},
		Volumes:  map[string]Volume{},
	}

	for _, name := range util.SortedKeys(profile.Deps) {
		dep := profile.Deps[name]
		image := rewriteImage(dep.Image, rewrite)

		svc := Service{
			Image:       image,
			Environment: dep.Env,
			Ports:       dep.Ports,
			DependsOn:   nil,
			Labels:      labels(manifest, profileName, name),
			Networks:    []string{"devx_default"},
		}

		if dep.Volume != "" {
			svc.Volumes = []string{dep.Volume}
			volumeName := strings.SplitN(dep.Volume, ":", 2)[0]
			if volumeName != "" {
				file.Volumes[volumeName] = Volume{}
			}
		}

		// Merge any provider-contributed configuration (e.g. healthcheck).
		if rewrite.DepFragments != nil {
			if frag, ok := rewrite.DepFragments[name]; ok && frag != nil && frag.Healthcheck != nil {
				svc.Healthcheck = frag.Healthcheck
			}
		}

		file.Services[name] = svc
	}

	if enableTelemetry {
		telemetryServices, telemetryVolumes := telemetryCompose(manifest, profileName, rewrite)
		for svcName, svc := range telemetryServices {
			if _, exists := file.Services[svcName]; exists {
				return "", fmt.Errorf("telemetry service name collision: %s", svcName)
			}
			file.Services[svcName] = svc
		}
		for volName := range telemetryVolumes {
			file.Volumes[volName] = Volume{}
		}
	}

	for _, name := range util.SortedKeys(profile.Services) {
		svc := profile.Services[name]
		if svc.IsProcess() {
			continue
		}
		service := Service{
			Image:       rewriteImage(svc.Image, rewrite),
			Ports:       svc.Ports,
			Environment: svc.Env,
			Command:     svc.Command,
			WorkingDir:  svc.Workdir,
			Volumes:     svc.Mount,
			DependsOn:   containerDeps(profile, svc.DependsOn),
			Labels:      labels(manifest, profileName, name),
			Networks:    []string{"devx_default"},
		}
		if len(containerDeps(profile, svc.DependsOn)) != len(svc.DependsOn) {
			service.ExtraHosts = []string{"host.docker.internal:host-gateway"}
		}

		if svc.Build != nil {
			service.Build = &Build{Context: svc.Build.Context, Dockerfile: svc.Build.Dockerfile}
			service.Image = ""
		}

		if svc.Health != nil && svc.Health.HttpGet != "" {
			service.Healthcheck = &Healthcheck{
				Test: []string{"CMD-SHELL", fmt.Sprintf("wget -qO- %s >/dev/null 2>&1 || exit 1", svc.Health.HttpGet)},
			}
			if svc.Health.Interval != "" {
				service.Healthcheck.Interval = svc.Health.Interval
			}
			if svc.Health.Retries > 0 {
				service.Healthcheck.Retries = svc.Health.Retries
			}
		}

		file.Services[name] = service
	}

	data, err := yaml.Marshal(file)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func labels(manifest *config.Manifest, profileName string, name string) map[string]string {
	return map[string]string{
		"devx.project": manifest.Project.Name,
		"devx.profile": profileName,
		"devx.service": name,
	}
}

func rewriteImage(image string, opts RewriteOptions) string {
	if image == "" {
		return image
	}

	rewritten := image
	if opts.RegistryPrefix != "" {
		rewritten = prefixRegistry(image, opts.RegistryPrefix)
	}
	if opts.Lockfile != nil {
		rewritten = lock.Apply(rewritten, opts.Lockfile)
	}
	return rewritten
}

func prefixRegistry(image string, prefix string) string {
	if strings.HasPrefix(image, prefix+"/") {
		return image
	}

	registry, remainder := splitRegistry(image)
	if registry != "" {
		return prefix + "/" + remainder
	}

	return prefix + "/" + image
}

func splitRegistry(image string) (string, string) {
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		return "", image
	}

	first := parts[0]
	if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
		return first, parts[1]
	}

	return "", image
}

func CollectImages(data []byte) ([]string, error) {
	var file File
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	var images []string
	for _, svc := range file.Services {
		if svc.Image != "" {
			images = append(images, svc.Image)
		}
	}

	return images, nil
}

func Normalize(data string) (string, error) {
	var file File
	if err := yaml.Unmarshal([]byte(data), &file); err != nil {
		return "", err
	}
	buf := &bytes.Buffer{}
	enc := yaml.NewEncoder(buf)
	enc.SetIndent(2)
	if err := enc.Encode(file); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// containerDeps drops dependsOn entries that refer to host process services,
// which are not part of the compose project.
func containerDeps(profile *config.Profile, deps []string) []string {
	var out []string
	for _, d := range deps {
		if s, ok := profile.Services[d]; ok && s.IsProcess() {
			continue
		}
		out = append(out, d)
	}
	return out
}
