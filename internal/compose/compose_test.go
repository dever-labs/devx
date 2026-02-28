package compose

import (
	"reflect"
	"testing"

	"github.com/dever-labs/devx/internal/config"
	"gopkg.in/yaml.v3"
)

func TestRenderCompose(t *testing.T) {
	manifest := &config.Manifest{
		Version:  1,
		Project:  config.Project{Name: "my-app", DefaultProfile: "local"},
		Registry: config.Registry{Prefix: "registry.local"},
	}

	profile := &config.Profile{
		Services: map[string]config.Service{
			"api": {
				Image:     "nginx:alpine",
				Ports:     []string{"8080:80"},
				DependsOn: []string{"db"},
			},
		},
		Deps: map[string]config.Dep{
			"db": {
				Kind:    "postgres",
				Version: "16",
				Env: map[string]string{
					"POSTGRES_PASSWORD": "postgres",
				},
				Ports:  []string{"5432:5432"},
				Volume: "db-data:/var/lib/postgresql/data",
			},
		},
	}

	out, err := Render(manifest, "local", profile, RewriteOptions{RegistryPrefix: "registry.local"}, false)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	var got File
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal output failed: %v", err)
	}

	expected := `version: "3.9"
services:
  api:
    image: registry.local/nginx:alpine
    ports:
      - "8080:80"
    depends_on:
      - db
    labels:
      devx.project: my-app
      devx.profile: local
      devx.service: api
    networks:
      - devx_default
  db:
    image: registry.local/postgres:16
    environment:
      POSTGRES_PASSWORD: postgres
    ports:
      - "5432:5432"
    volumes:
      - "db-data:/var/lib/postgresql/data"
    labels:
      devx.project: my-app
      devx.profile: local
      devx.service: db
    networks:
      - devx_default
networks:
  devx_default: {}
volumes:
  db-data: {}
`

	var want File
	if err := yaml.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatalf("unmarshal expected failed: %v", err)
	}

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("compose output mismatch\nGot: %#v\nWant: %#v", got, want)
	}
}
