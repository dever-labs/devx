package compose

import (
	"fmt"

	"github.com/dever-labs/devx/internal/config"
)

type Asset struct {
	Path    string
	Content []byte
}

const (
	grafanaImage    = "grafana/grafana:10.4.3"
	lokiImage       = "grafana/loki:2.9.2"
	prometheusImage = "prom/prometheus:v2.50.1"
	telemetryName   = "devx-telemetry"
)

func TelemetryAssets(enable bool) []Asset {
	if !enable {
		return nil
	}

	return []Asset{
		{
			Path:    "telemetry/loki-config.yaml",
			Content: []byte(lokiConfig()),
		},
		{
			Path:    "telemetry/prometheus.yml",
			Content: []byte(prometheusConfig(telemetryName)),
		},
		{
			Path:    "telemetry/grafana/provisioning/datasources/devx.yaml",
			Content: []byte(grafanaDatasourceConfig(telemetryName)),
		},
	}
}

func telemetryCompose(manifest *config.Manifest, profileName string, rewrite RewriteOptions) (map[string]Service, map[string]Volume) {
	services := map[string]Service{}
	volumes := map[string]Volume{}

	grafanaName := telemetryName + "-grafana"
	lokiName := telemetryName + "-loki"
	promName := telemetryName + "-prometheus"

	grafanaPorts := []string{"3000"}

	services[grafanaName] = Service{
		Image:      rewriteImage(grafanaImage, rewrite),
		Ports:      grafanaPorts,
		DependsOn:  []string{lokiName, promName},
		Labels:     labels(manifest, profileName, grafanaName),
		Networks:   []string{"devx_default"},
		Environment: map[string]string{
			"GF_AUTH_ANONYMOUS_ENABLED":   "true",
			"GF_AUTH_ANONYMOUS_ORG_ROLE":  "Admin",
			"GF_USERS_DEFAULT_THEME":      "light",
			"GF_ANALYTICS_REPORTING_ENABLED": "false",
		},
		Volumes: []string{
			telemetryName + "-grafana-data:/var/lib/grafana",
			"./telemetry/grafana/provisioning/datasources/devx.yaml:/etc/grafana/provisioning/datasources/devx.yaml:ro",
		},
	}

	services[lokiName] = Service{
		Image:      rewriteImage(lokiImage, rewrite),
		Labels:     labels(manifest, profileName, lokiName),
		Networks:   []string{"devx_default"},
		Command:    []string{"-config.file=/etc/loki/local-config.yaml"},
		Volumes: []string{
			telemetryName + "-loki-data:/loki",
			"./telemetry/loki-config.yaml:/etc/loki/local-config.yaml:ro",
		},
	}

	services[promName] = Service{
		Image:    rewriteImage(prometheusImage, rewrite),
		Labels:   labels(manifest, profileName, promName),
		Networks: []string{"devx_default"},
		Volumes: []string{
			telemetryName + "-prometheus-data:/prometheus",
			"./telemetry/prometheus.yml:/etc/prometheus/prometheus.yml:ro",
		},
	}

	volumes[telemetryName+"-grafana-data"] = Volume{}
	volumes[telemetryName+"-loki-data"] = Volume{}
	volumes[telemetryName+"-prometheus-data"] = Volume{}

	return services, volumes
}

func lokiConfig() string {
	return `auth_enabled: false

server:
  http_listen_port: 3100

common:
  instance_addr: 127.0.0.1
  path_prefix: /loki
  storage:
    filesystem:
      chunks_directory: /loki/chunks
      rules_directory: /loki/rules
  replication_factor: 1
  ring:
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: 2024-01-01
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h

ruler:
  alertmanager_url: http://localhost:9093
`
}

func prometheusConfig(depName string) string {
	return fmt.Sprintf(`global:
  scrape_interval: 15s

scrape_configs:
  - job_name: "prometheus"
    static_configs:
			- targets: ["%s:9090"]
  - job_name: "loki"
    static_configs:
			- targets: ["%s-loki:3100"]
`, depName+"-prometheus", depName)
}

func grafanaDatasourceConfig(depName string) string {
	return `apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
		url: http://` + depName + `-prometheus:9090
    isDefault: true
  - name: Loki
    type: loki
    access: proxy
		url: http://` + depName + `-loki:3100
`
}
