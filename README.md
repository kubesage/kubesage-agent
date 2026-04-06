# kubesage-agent

[![CI](https://github.com/kubesage/kubesage-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/kubesage/kubesage-agent/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/kubesage/kubesage-agent)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## Overview

KubeSage cluster monitoring agent for Kubernetes. Collects resource metrics (CPU, memory, pods), node health, and workload status from Kubernetes clusters and reports to the KubeSage platform or any compatible endpoint.

## Features

- Kubelet metrics collection (CPU, memory per node)
- Kubernetes informer-based resource tracking (deployments, statefulsets, daemonsets)
- OTLP/gRPC metrics export to any OpenTelemetry-compatible collector
- Health and readiness endpoints (`/healthz`, `/readyz`)
- Configurable via environment variables
- Lightweight Alpine-based Docker image
- Helm chart for easy Kubernetes deployment

## Installation

### Helm

```bash
helm install kubesage-agent ./charts/kubesage-agent \
  --set env.KUBESAGE_API_URL=https://api.kubesage.com \
  --set env.CLUSTER_ID=my-cluster
```

### Docker

```bash
docker run ghcr.io/kubesage/kubesage-agent:latest
```

### Binary

```bash
go install github.com/kubesage/kubesage-agent/cmd/agent@latest
```

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `KUBESAGE_API_URL` | KubeSage API endpoint | - |
| `CLUSTER_ID` | Cluster identifier | - |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint | `localhost:4317` |
| `COLLECTION_INTERVAL` | Metrics collection interval | `30s` |
| `LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |
| `HEALTH_PORT` | Health endpoint port | `8080` |
| `CERT_DIR` | Directory for TLS certificates (mTLS) | `/etc/kubesage/certs` |

## Development

```bash
# Build
make build

# Run tests
make test

# Lint
make lint

# Build Docker image
make docker

# Clean build artifacts
make clean
```

## Architecture

```
cmd/agent/main.go          # Entrypoint: config, K8s client, OTel setup, signal handling
internal/
  config/                  # Environment-based configuration with pflag
  collector/               # Kubelet scraper + K8s informer-based metric collection
  exporter/                # OTLP/gRPC meter provider with optional mTLS
  health/                  # HTTP health/readiness server
  metrics/                 # OTel instrument definitions and K8s resource attributes
  api/                     # REST client for KubeSage platform API
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
