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
	alloyImage      = "grafana/alloy:v1.1.1"
	cAdvisorImage   = "gcr.io/cadvisor/cadvisor:v0.49.1"
	dockerMetaImage = "python:3.12-alpine"
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
			Path:    "telemetry/alloy-config.alloy",
			Content: []byte(alloyConfig(telemetryName)),
		},
		{
			Path:    "telemetry/grafana/provisioning/datasources/devx.yaml",
			Content: []byte(grafanaDatasourceConfig(telemetryName)),
		},
		{
			Path:    "telemetry/grafana/provisioning/dashboards/devx.yaml",
			Content: []byte(grafanaDashboardProvisioningConfig()),
		},
		{
			Path:    "telemetry/grafana/dashboards/logs.json",
			Content: []byte(grafanaLogsDashboard()),
		},
		{
			Path:    "telemetry/grafana/dashboards/resources.json",
			Content: []byte(grafanaContainerResourcesDashboard()),
		},
		{
			Path:    "telemetry/grafana/dashboards/log-analytics.json",
			Content: []byte(grafanaLogAnalyticsDashboard()),
		},
		{
			Path:    "telemetry/grafana/dashboards/health.json",
			Content: []byte(grafanaServiceHealthDashboard()),
		},
		{
			Path:    "telemetry/docker-meta-exporter.py",
			Content: []byte(dockerMetaExporterScript()),
		},
	}
}

func telemetryCompose(manifest *config.Manifest, profileName string, rewrite RewriteOptions) (map[string]Service, map[string]Volume) {
	services := map[string]Service{}
	volumes := map[string]Volume{}

	grafanaName := telemetryName + "-grafana"
	lokiName := telemetryName + "-loki"
	promName := telemetryName + "-prometheus"
	alloyName := telemetryName + "-alloy"
	cAdvisorName := telemetryName + "-cadvisor"

	grafanaPorts := []string{"3000"}

	services[grafanaName] = Service{
		Image:     rewriteImage(grafanaImage, rewrite),
		Ports:     grafanaPorts,
		DependsOn: []string{lokiName, promName},
		Labels:    labels(manifest, profileName, grafanaName),
		Networks:  []string{"devx_default"},
		Environment: map[string]string{
			"GF_AUTH_ANONYMOUS_ENABLED":      "true",
			"GF_AUTH_ANONYMOUS_ORG_ROLE":     "Admin",
			"GF_USERS_DEFAULT_THEME":         "light",
			"GF_ANALYTICS_REPORTING_ENABLED": "false",
		},
		Volumes: []string{
			telemetryName + "-grafana-data:/var/lib/grafana",
			"./telemetry/grafana/provisioning/datasources/devx.yaml:/etc/grafana/provisioning/datasources/devx.yaml:ro",
			"./telemetry/grafana/provisioning/dashboards/devx.yaml:/etc/grafana/provisioning/dashboards/devx.yaml:ro",
			"./telemetry/grafana/dashboards:/var/lib/grafana/dashboards:ro",
		},
	}

	services[lokiName] = Service{
		Image:    rewriteImage(lokiImage, rewrite),
		Labels:   labels(manifest, profileName, lokiName),
		Networks: []string{"devx_default"},
		Command:  []string{"-config.file=/etc/loki/local-config.yaml"},
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

	services[alloyName] = Service{
		Image:    rewriteImage(alloyImage, rewrite),
		Labels:   labels(manifest, profileName, alloyName),
		Networks: []string{"devx_default"},
		Command:  []string{"run", "--server.http.listen-addr=0.0.0.0:12345", "/etc/alloy/config.alloy"},
		Volumes: []string{
			"./telemetry/alloy-config.alloy:/etc/alloy/config.alloy:ro",
			"/var/run/docker.sock:/var/run/docker.sock:ro",
		},
	}

	// cAdvisor exposes per-container CPU, memory, and network metrics.
	// /var/run must be rw so cAdvisor can connect to the Docker socket and resolve container names.
	services[cAdvisorName] = Service{
		Image:      rewriteImage(cAdvisorImage, rewrite),
		Labels:     labels(manifest, profileName, cAdvisorName),
		Networks:   []string{"devx_default"},
		Privileged: true,
		Volumes: []string{
			"/:/rootfs:ro",
			"/var/run:/var/run:rw",
			"/sys:/sys:ro",
			"/var/lib/docker/:/var/lib/docker:ro",
			"/dev/disk/:/dev/disk:ro",
		},
	}

	// docker-meta-exporter queries the Docker API and exposes container ID→name/label
	// mappings as docker_container_info Prometheus metrics, enabling group_left joins
	// with cAdvisor metrics (which only carry the raw container ID in their `id` label).
	dockerMetaName := telemetryName + "-docker-meta"
	services[dockerMetaName] = Service{
		Image:    rewriteImage(dockerMetaImage, rewrite),
		Labels:   labels(manifest, profileName, dockerMetaName),
		Networks: []string{"devx_default"},
		Command:  []string{"python", "/app/exporter.py"},
		Volumes: []string{
			"./telemetry/docker-meta-exporter.py:/app/exporter.py:ro",
			"/var/run/docker.sock:/var/run/docker.sock:ro",
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
  - job_name: "cadvisor"
    static_configs:
      - targets: ["%s-cadvisor:8080"]
  - job_name: "docker-meta"
    static_configs:
      - targets: ["%s-docker-meta:9101"]
`, depName+"-prometheus", depName, depName, depName)
}

func alloyConfig(depName string) string {
	return fmt.Sprintf(`discovery.docker "containers" {
  host             = "unix:///var/run/docker.sock"
  refresh_interval = "5s"
}

discovery.relabel "containers" {
  targets = discovery.docker.containers.targets

  rule {
    source_labels = ["__meta_docker_container_name"]
    regex         = "/?(.+)"
    target_label  = "container"
  }
  rule {
    source_labels = ["__meta_docker_container_log_stream"]
    target_label  = "logstream"
  }
  rule {
    source_labels = ["__meta_docker_container_label_com_docker_compose_project"]
    target_label  = "compose_project"
  }
  rule {
    source_labels = ["__meta_docker_container_label_com_docker_compose_service"]
    target_label  = "compose_service"
  }
  rule {
    source_labels = ["__meta_docker_container_label_com_docker_compose_project"]
    regex         = ".+"
    action        = "keep"
  }
}

loki.source.docker "containers" {
  host             = "unix:///var/run/docker.sock"
  targets          = discovery.relabel.containers.output
  forward_to       = [loki.write.local.receiver]
  refresh_interval = "5s"
}

loki.write "local" {
  endpoint {
    url = "http://%s-loki:3100/loki/api/v1/push"
  }
}
`, depName)
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

func grafanaDashboardProvisioningConfig() string {
	return `apiVersion: 1

providers:
  - name: devx
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /var/lib/grafana/dashboards
      foldersFromFilesStructure: false
`
}

func grafanaLogsDashboard() string {
	return `{
  "__inputs": [],
  "__requires": [],
  "annotations": { "list": [] },
  "editable": true,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "panels": [
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 },
      "id": 10,
      "title": "Volume",
      "type": "row"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "fillOpacity": 8, "lineWidth": 2 }
        },
        "overrides": []
      },
      "gridPos": { "h": 7, "w": 24, "x": 0, "y": 1 },
      "id": 1,
      "options": {
        "legend": { "calcs": ["sum"], "displayMode": "table", "placement": "right" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum by (compose_service) (count_over_time({compose_service=~\"$service\"}[$__interval]))",
          "legendFormat": "{{compose_service}}",
          "queryType": "range",
          "refId": "A"
        }
      ],
      "title": "Log Rate by Service",
      "type": "timeseries"
    },
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 8 },
      "id": 11,
      "title": "Stream",
      "type": "row"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "gridPos": { "h": 20, "w": 24, "x": 0, "y": 9 },
      "id": 2,
      "options": {
        "dedupStrategy": "none",
        "enableLogDetails": true,
        "prettifyLogMessage": false,
        "showCommonLabels": false,
        "showLabels": true,
        "showTime": true,
        "sortOrder": "Descending",
        "wrapLogMessage": false
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "{compose_service=~\"$service\"} |~ \"(?i)$search\"",
          "queryType": "range",
          "refId": "A"
        }
      ],
      "title": "Log Stream",
      "type": "logs"
    }
  ],
  "refresh": "5s",
  "schemaVersion": 38,
  "tags": ["devx", "logs"],
  "templating": {
    "list": [
      {
        "current": {},
        "hide": 2,
        "name": "DS_LOKI",
        "options": [],
        "query": "loki",
        "refresh": 1,
        "type": "datasource"
      },
      {
        "current": { "selected": true, "text": ["All"], "value": ["$__all"] },
        "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
        "definition": "label_values(compose_service)",
        "hide": 0,
        "includeAll": true,
        "multi": true,
        "name": "service",
        "options": [],
        "query": "label_values(compose_service)",
        "refresh": 2,
        "sort": 1,
        "type": "query",
        "label": "Service"
      },
      {
        "current": { "selected": false, "text": "", "value": "" },
        "hide": 0,
        "name": "search",
        "options": [{ "selected": false, "text": "", "value": "" }],
        "type": "textbox",
        "label": "Log filter (regex)"
      }
    ]
  },
  "time": { "from": "now-1h", "to": "now" },
  "timepicker": {},
  "timezone": "browser",
  "title": "Container Logs",
  "uid": "devx-logs",
  "version": 2
}
`
}

func grafanaContainerResourcesDashboard() string {
	return `{
  "__inputs": [],
  "__requires": [],
  "annotations": { "list": [] },
  "editable": true,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "panels": [
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 },
      "id": 30,
      "title": "Log Activity",
      "type": "row"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "fillOpacity": 8, "lineWidth": 2 }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 1 },
      "id": 10,
      "options": {
        "legend": { "calcs": ["sum"], "displayMode": "table", "placement": "bottom" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum by (container) (count_over_time({compose_service=~\"$service\"}[$__interval]))",
          "legendFormat": "{{container}}",
          "queryType": "range",
          "refId": "A"
        }
      ],
      "title": "Log Rate",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "fillOpacity": 8, "lineWidth": 2 }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 1 },
      "id": 11,
      "options": {
        "legend": { "calcs": ["sum"], "displayMode": "table", "placement": "bottom" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum by (compose_service) (count_over_time({compose_service=~\"$service\"} |~ \"(?i)(error|exception|fatal|panic)\" [$__interval]))",
          "legendFormat": "{{compose_service}} errors",
          "queryType": "range",
          "refId": "A"
        }
      ],
      "title": "Error Rate",
      "type": "timeseries"
    },
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 9 },
      "id": 20,
      "title": "CPU (requires cAdvisor + docker-meta)",
      "type": "row"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "unit": "percent",
          "custom": { "fillOpacity": 8, "lineWidth": 2 }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 24, "x": 0, "y": 10 },
      "id": 1,
      "options": {
        "legend": { "calcs": ["mean", "max"], "displayMode": "table", "placement": "right" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
          "expr": "sum by (compose_service) (rate(container_cpu_usage_seconds_total{id=~\"/docker/.+\"}[$__rate_interval]) * on(id) group_left(compose_service) docker_container_info{compose_service=~\"$service\"}) * 100",
          "legendFormat": "{{compose_service}}",
          "refId": "A"
        }
      ],
      "title": "CPU Usage %",
      "type": "timeseries"
    },
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 18 },
      "id": 21,
      "title": "Memory (requires cAdvisor)",
      "type": "row"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "unit": "bytes",
          "custom": { "fillOpacity": 8, "lineWidth": 2 }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 19 },
      "id": 2,
      "options": {
        "legend": { "calcs": ["mean", "max"], "displayMode": "table", "placement": "bottom" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
          "expr": "sum by (compose_service) (container_memory_usage_bytes{id=~\"/docker/.+\"} * on(id) group_left(compose_service) docker_container_info{compose_service=~\"$service\"})",
          "legendFormat": "{{compose_service}}",
          "refId": "A"
        }
      ],
      "title": "Memory Usage",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "unit": "bytes",
          "custom": { "fillOpacity": 8, "lineWidth": 2 }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 19 },
      "id": 3,
      "options": {
        "legend": { "calcs": ["mean", "max"], "displayMode": "table", "placement": "bottom" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
          "expr": "sum by (compose_service) (container_memory_cache{id=~\"/docker/.+\"} * on(id) group_left(compose_service) docker_container_info{compose_service=~\"$service\"})",
          "legendFormat": "{{compose_service}} (cache)",
          "refId": "A"
        }
      ],
      "title": "Memory Cache",
      "type": "timeseries"
    },
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 27 },
      "id": 22,
      "title": "Network (requires cAdvisor)",
      "type": "row"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "unit": "Bps",
          "custom": { "fillOpacity": 8, "lineWidth": 2 }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 28 },
      "id": 4,
      "options": {
        "legend": { "calcs": ["mean"], "displayMode": "table", "placement": "bottom" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
          "expr": "sum by (compose_service) (rate(docker_container_network_rx_bytes_total{compose_service=~\"$service\", compose_service!=\"\"}[$__rate_interval]))",
          "legendFormat": "{{compose_service}}",
          "refId": "A"
        }
      ],
      "title": "Network Rx (per container)",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "unit": "Bps",
          "custom": { "fillOpacity": 8, "lineWidth": 2 }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 28 },
      "id": 5,
      "options": {
        "legend": { "calcs": ["mean"], "displayMode": "table", "placement": "bottom" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
          "expr": "sum by (compose_service) (rate(docker_container_network_tx_bytes_total{compose_service=~\"$service\", compose_service!=\"\"}[$__rate_interval]))",
          "legendFormat": "{{compose_service}}",
          "refId": "A"
        }
      ],
      "title": "Network Tx (per container)",
      "type": "timeseries"
    }
  ],
  "refresh": "10s",
  "schemaVersion": 38,
  "tags": ["devx", "metrics", "resources"],
  "templating": {
    "list": [
      {
        "current": {},
        "hide": 2,
        "name": "DS_PROMETHEUS",
        "options": [],
        "query": "prometheus",
        "refresh": 1,
        "type": "datasource"
      },
      {
        "current": {},
        "hide": 2,
        "name": "DS_LOKI",
        "options": [],
        "query": "loki",
        "refresh": 1,
        "type": "datasource"
      },
      {
        "current": { "selected": true, "text": ["All"], "value": ["$__all"] },
        "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
        "definition": "label_values(compose_service)",
        "hide": 0,
        "includeAll": true,
        "multi": true,
        "name": "service",
        "options": [],
        "query": "label_values(compose_service)",
        "refresh": 2,
        "sort": 1,
        "type": "query",
        "label": "Service"
      }
    ]
  },
  "time": { "from": "now-30m", "to": "now" },
  "timepicker": {},
  "timezone": "browser",
  "title": "Container Resources",
  "uid": "devx-resources",
  "version": 4
}
`
}

func grafanaLogAnalyticsDashboard() string {
	return `{
  "__inputs": [],
  "__requires": [],
  "annotations": { "list": [] },
  "editable": true,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "panels": [
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "unit": "short",
          "thresholds": { "mode": "absolute", "steps": [
            { "color": "green", "value": null },
            { "color": "red", "value": 1 }
          ]}
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 4, "x": 0, "y": 0 },
      "id": 1,
      "options": {
        "colorMode": "background",
        "graphMode": "none",
        "justifyMode": "center",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["sum"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum(count_over_time({compose_project=~\".+\"} |~ \"(?i)(error|exception|fatal|panic)\" [$__range]))",
          "queryType": "instant",
          "refId": "A"
        }
      ],
      "title": "Errors",
      "type": "stat"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "unit": "short",
          "thresholds": { "mode": "absolute", "steps": [
            { "color": "green", "value": null },
            { "color": "yellow", "value": 1 }
          ]}
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 4, "x": 4, "y": 0 },
      "id": 2,
      "options": {
        "colorMode": "background",
        "graphMode": "none",
        "justifyMode": "center",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["sum"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum(count_over_time({compose_project=~\".+\"} |~ \"(?i)warn\" [$__range]))",
          "queryType": "instant",
          "refId": "A"
        }
      ],
      "title": "Warnings",
      "type": "stat"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "unit": "short",
          "thresholds": { "mode": "absolute", "steps": [{ "color": "blue", "value": null }]}
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 4, "x": 8, "y": 0 },
      "id": 3,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "center",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["sum"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum(count_over_time({compose_project=~\".+\"}[$__range]))",
          "queryType": "instant",
          "refId": "A"
        }
      ],
      "title": "Total Log Lines",
      "type": "stat"
    },
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 4 },
      "id": 20,
      "title": "Log Volume",
      "type": "row"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "fillOpacity": 30, "lineWidth": 1, "stacking": { "group": "A", "mode": "normal" } }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 24, "x": 0, "y": 5 },
      "id": 4,
      "options": {
        "legend": { "calcs": ["sum"], "displayMode": "table", "placement": "right" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum by (compose_service) (count_over_time({compose_project=~\".+\"}[$__interval]))",
          "legendFormat": "{{compose_service}}",
          "queryType": "range",
          "refId": "A"
        }
      ],
      "title": "Log Volume by Service (stacked)",
      "type": "timeseries"
    },
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 13 },
      "id": 21,
      "title": "Error Analysis",
      "type": "row"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "fillOpacity": 8, "lineWidth": 2 }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 24, "x": 0, "y": 14 },
      "id": 5,
      "options": {
        "legend": { "calcs": ["sum", "max"], "displayMode": "table", "placement": "right" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum by (compose_service) (count_over_time({compose_project=~\".+\"} |~ \"(?i)(error|exception|fatal|panic)\" [$__interval]))",
          "legendFormat": "{{compose_service}} errors",
          "queryType": "range",
          "refId": "A"
        }
      ],
      "title": "Error Rate by Service",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "gridPos": { "h": 14, "w": 24, "x": 0, "y": 22 },
      "id": 6,
      "options": {
        "dedupStrategy": "none",
        "enableLogDetails": true,
        "prettifyLogMessage": false,
        "showCommonLabels": false,
        "showLabels": true,
        "showTime": true,
        "sortOrder": "Descending",
        "wrapLogMessage": false
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "{compose_project=~\".+\"} |~ \"(?i)(error|exception|fatal|panic)\"",
          "queryType": "range",
          "refId": "A"
        }
      ],
      "title": "Error / Exception Log Stream",
      "type": "logs"
    }
  ],
  "refresh": "30s",
  "schemaVersion": 38,
  "tags": ["devx", "logs", "analytics"],
  "templating": {
    "list": [
      {
        "current": {},
        "hide": 2,
        "name": "DS_LOKI",
        "options": [],
        "query": "loki",
        "refresh": 1,
        "type": "datasource"
      }
    ]
  },
  "time": { "from": "now-1h", "to": "now" },
  "timepicker": {},
  "timezone": "browser",
  "title": "Log Analytics",
  "uid": "devx-log-analytics",
  "version": 1
}
`
}

func grafanaServiceHealthDashboard() string {
	return `{
  "__inputs": [],
  "__requires": [],
  "annotations": { "list": [] },
  "editable": true,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "panels": [
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "unit": "short",
          "thresholds": { "mode": "absolute", "steps": [
            { "color": "red", "value": null },
            { "color": "green", "value": 1 }
          ]}
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 6, "x": 0, "y": 0 },
      "id": 1,
      "options": {
        "colorMode": "background",
        "graphMode": "none",
        "justifyMode": "center",
        "reduceOptions": { "calcs": ["lastNotNull"] }
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "count(sum by (compose_service) (count_over_time({compose_project=~\".+\"}[5m])))",
          "queryType": "instant",
          "refId": "A"
        }
      ],
      "title": "Active Services",
      "type": "stat"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "unit": "short",
          "thresholds": { "mode": "absolute", "steps": [
            { "color": "green", "value": null },
            { "color": "red", "value": 1 }
          ]}
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 6, "x": 6, "y": 0 },
      "id": 2,
      "options": {
        "colorMode": "background",
        "graphMode": "none",
        "justifyMode": "center",
        "reduceOptions": { "calcs": ["sum"] }
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum(count_over_time({compose_project=~\".+\"} |~ \"(?i)(error|exception|fatal|panic)\" [$__range]))",
          "queryType": "instant",
          "refId": "A"
        }
      ],
      "title": "Errors",
      "type": "stat"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "unit": "short",
          "thresholds": { "mode": "absolute", "steps": [
            { "color": "green", "value": null },
            { "color": "yellow", "value": 1 }
          ]}
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 6, "x": 12, "y": 0 },
      "id": 3,
      "options": {
        "colorMode": "background",
        "graphMode": "none",
        "justifyMode": "center",
        "reduceOptions": { "calcs": ["sum"] }
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum(count_over_time({compose_project=~\".+\"} |~ \"(?i)warn\" [$__range]))",
          "queryType": "instant",
          "refId": "A"
        }
      ],
      "title": "Warnings",
      "type": "stat"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "unit": "short",
          "thresholds": { "mode": "absolute", "steps": [{ "color": "blue", "value": null }]}
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 6, "x": 18, "y": 0 },
      "id": 4,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "center",
        "reduceOptions": { "calcs": ["sum"] }
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "sum(count_over_time({compose_project=~\".+\"}[$__range]))",
          "queryType": "instant",
          "refId": "A"
        }
      ],
      "title": "Log Lines",
      "type": "stat"
    },
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 4 },
      "id": 20,
      "title": "Top Services",
      "type": "row"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "continuous-BlPu" },
          "unit": "short",
          "thresholds": { "mode": "absolute", "steps": [{ "color": "blue", "value": null }]}
        },
        "overrides": []
      },
      "gridPos": { "h": 10, "w": 12, "x": 0, "y": 5 },
      "id": 5,
      "options": {
        "displayMode": "gradient",
        "minVizHeight": 10,
        "minVizWidth": 0,
        "orientation": "horizontal",
        "reduceOptions": { "calcs": ["sum"], "fields": "", "values": false },
        "showUnfilled": true,
        "valueMode": "color"
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "topk(8, sum by (compose_service) (count_over_time({compose_project=~\".+\"}[$__range])))",
          "legendFormat": "{{compose_service}}",
          "queryType": "instant",
          "refId": "A"
        }
      ],
      "title": "Top by Log Volume",
      "type": "bargauge"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "continuous-RdYlGr" },
          "unit": "short",
          "thresholds": { "mode": "absolute", "steps": [
            { "color": "green", "value": null },
            { "color": "red", "value": 1 }
          ]}
        },
        "overrides": []
      },
      "gridPos": { "h": 10, "w": 12, "x": 12, "y": 5 },
      "id": 6,
      "options": {
        "displayMode": "gradient",
        "minVizHeight": 10,
        "minVizWidth": 0,
        "orientation": "horizontal",
        "reduceOptions": { "calcs": ["sum"], "fields": "", "values": false },
        "showUnfilled": true,
        "valueMode": "color"
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "topk(8, sum by (compose_service) (count_over_time({compose_project=~\".+\"} |~ \"(?i)(error|exception|fatal|panic)\" [$__range])))",
          "legendFormat": "{{compose_service}}",
          "queryType": "instant",
          "refId": "A"
        }
      ],
      "title": "Top by Error Count",
      "type": "bargauge"
    },
    {
      "collapsed": true,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 15 },
      "id": 25,
      "panels": [
        {
          "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
          "fieldConfig": {
            "defaults": {
              "color": { "mode": "continuous-GrYlRd" },
              "unit": "percent",
              "thresholds": { "mode": "absolute", "steps": [
                { "color": "green", "value": null },
                { "color": "yellow", "value": 50 },
                { "color": "red", "value": 80 }
              ]}
            },
            "overrides": []
          },
          "gridPos": { "h": 10, "w": 12, "x": 0, "y": 16 },
          "id": 26,
          "options": {
            "displayMode": "gradient",
            "orientation": "horizontal",
            "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
            "showUnfilled": true,
            "valueMode": "color"
          },
          "targets": [
            {
              "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
              "expr": "topk(8, sum by (compose_service) (rate(container_cpu_usage_seconds_total{id=~\"/docker/.+\"}[1m]) * on(id) group_left(compose_service) docker_container_info) * 100)",
              "legendFormat": "{{compose_service}}",
              "refId": "A"
            }
          ],
          "title": "Top CPU Consumers",
          "type": "bargauge"
        },
        {
          "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
          "fieldConfig": {
            "defaults": {
              "color": { "mode": "continuous-BlYlRd" },
              "unit": "bytes",
              "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }]}
            },
            "overrides": []
          },
          "gridPos": { "h": 10, "w": 12, "x": 12, "y": 16 },
          "id": 27,
          "options": {
            "displayMode": "gradient",
            "orientation": "horizontal",
            "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
            "showUnfilled": true,
            "valueMode": "color"
          },
          "targets": [
            {
              "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
              "expr": "topk(8, sum by (compose_service) (container_memory_usage_bytes{id=~\"/docker/.+\"} * on(id) group_left(compose_service) docker_container_info))",
              "legendFormat": "{{compose_service}}",
              "refId": "A"
            }
          ],
          "title": "Top Memory Consumers",
          "type": "bargauge"
        }
      ],
      "title": "Container Metrics (requires cAdvisor)",
      "type": "row"
    },
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 16 },
      "id": 22,
      "title": "Recent Errors",
      "type": "row"
    },
    {
      "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
      "gridPos": { "h": 10, "w": 24, "x": 0, "y": 17 },
      "id": 7,
      "options": {
        "dedupStrategy": "none",
        "enableLogDetails": true,
        "prettifyLogMessage": false,
        "showCommonLabels": false,
        "showLabels": true,
        "showTime": true,
        "sortOrder": "Descending",
        "wrapLogMessage": false
      },
      "targets": [
        {
          "datasource": { "type": "loki", "uid": "${DS_LOKI}" },
          "expr": "{compose_project=~\".+\"} |~ \"(?i)(error|exception|fatal|panic)\"",
          "queryType": "range",
          "refId": "A"
        }
      ],
      "title": "Recent Errors (all services)",
      "type": "logs"
    }
  ],
  "refresh": "15s",
  "schemaVersion": 38,
  "tags": ["devx", "health"],
  "templating": {
    "list": [
      {
        "current": {},
        "hide": 2,
        "name": "DS_PROMETHEUS",
        "options": [],
        "query": "prometheus",
        "refresh": 1,
        "type": "datasource"
      },
      {
        "current": {},
        "hide": 2,
        "name": "DS_LOKI",
        "options": [],
        "query": "loki",
        "refresh": 1,
        "type": "datasource"
      }
    ]
  },
  "time": { "from": "now-15m", "to": "now" },
  "timepicker": {},
  "timezone": "browser",
  "title": "Service Health",
  "uid": "devx-health",
  "version": 3
}
`
}

// dockerMetaExporterScript returns a Python 3 stdlib HTTP server that queries
// the Docker API for container metadata AND per-container network stats.
// Stats are collected in parallel (one goroutine per container) and cached,
// so the /metrics endpoint always responds instantly.
func dockerMetaExporterScript() string {
	return `import json, socket, threading, time
from http.server import HTTPServer, BaseHTTPRequestHandler

def docker_get(path):
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.settimeout(10)
    s.connect('/var/run/docker.sock')
    s.sendall(('GET ' + path + ' HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n').encode())
    data = b''
    while True:
        chunk = s.recv(65536)
        if not chunk:
            break
        data += chunk
    s.close()
    header, _, body = data.partition(b'\r\n\r\n')
    if b'chunked' in header.lower():
        decoded = b''
        while body:
            crlf = body.find(b'\r\n')
            if crlf == -1:
                break
            size = int(body[:crlf], 16)
            if size == 0:
                break
            decoded += body[crlf+2:crlf+2+size]
            body = body[crlf+2+size+2:]
        body = decoded
    return json.loads(body)

def esc(v):
    return str(v).replace('\\', '\\\\').replace('"', '\\"').replace('\n', '\\n')

def fetch_stats(cid, out, idx):
    try:
        # one-shot=true skips the 1-second CPU measurement wait
        out[idx] = docker_get('/containers/' + cid + '/stats?stream=false&one-shot=true')
    except Exception:
        try:
            out[idx] = docker_get('/containers/' + cid + '/stats?stream=false')
        except Exception:
            out[idx] = None

_cache = {'body': b'', 'lock': threading.Lock()}

def build():
    try:
        containers = docker_get('/containers/json')
    except Exception as e:
        return ('# error: ' + str(e) + '\n').encode()

    stats_out = [None] * len(containers)
    threads = [threading.Thread(target=fetch_stats, args=(c['Id'], stats_out, i), daemon=True)
               for i, c in enumerate(containers)]
    for t in threads: t.start()
    for t in threads: t.join(timeout=12)

    lines = [
        '# HELP docker_container_info Container metadata for group_left joins',
        '# TYPE docker_container_info gauge',
        '# HELP docker_container_network_rx_bytes_total Cumulative bytes received per container interface',
        '# TYPE docker_container_network_rx_bytes_total counter',
        '# HELP docker_container_network_tx_bytes_total Cumulative bytes transmitted per container interface',
        '# TYPE docker_container_network_tx_bytes_total counter',
    ]
    for i, c in enumerate(containers):
        full_id = '/docker/' + c['Id']
        name = c['Names'][0].lstrip('/') if c.get('Names') else ''
        lbl  = c.get('Labels') or {}
        svc  = lbl.get('com.docker.compose.service', '')
        proj = lbl.get('com.docker.compose.project', '')
        base = ('id="' + esc(full_id) + '",name="' + esc(name) +
                '",compose_service="' + esc(svc) + '",compose_project="' + esc(proj) + '"')
        lines.append('docker_container_info{' + base + '} 1')
        st = stats_out[i]
        if st:
            for iface, net in (st.get('networks') or {}).items():
                ilbl = base + ',interface="' + esc(iface) + '"'
                lines.append('docker_container_network_rx_bytes_total{' + ilbl + '} ' + str(net.get('rx_bytes', 0)))
                lines.append('docker_container_network_tx_bytes_total{' + ilbl + '} ' + str(net.get('tx_bytes', 0)))
    return ('\n'.join(lines) + '\n').encode()

def refresh_loop():
    while True:
        body = build()
        with _cache['lock']:
            _cache['body'] = body
        time.sleep(15)

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != '/metrics':
            self.send_response(404); self.end_headers(); return
        with _cache['lock']:
            body = _cache['body']
        self.send_response(200)
        self.send_header('Content-Type', 'text/plain; version=0.0.4')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a): pass

_cache['body'] = build()
threading.Thread(target=refresh_loop, daemon=True).start()
HTTPServer(('', 9101), Handler).serve_forever()
`
}
