# kubesage-agent

[![CI](https://github.com/kubesage/kubesage-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/kubesage/kubesage-agent/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/kubesage/kubesage-agent)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## Overview

KubeSage cluster monitoring agent for Kubernetes. Collects resource metrics (CPU, memory, pods), node health, and workload status from Kubernetes clusters and reports to the KubeSage platform or any compatible endpoint.

## Local Development Rule

All install / build / dev / test commands run inside containers. Never on host. See the canonical workspace rule at: `../../CLAUDE.md` > Local Development Rule (workspace root, when this repo is cloned alongside the kubesage workspace).

Container equivalents:

| Verb | Container command |
|------|-------------------|
| `go mod download` | `make install` |
| `go run ./cmd/agent` | `make dev` (with HMR via air) |
| `go build` | `make build` |
| `go test` | `make test` |

Bypass once: `KUBESAGE_ALLOW_HOST=1 go test ./...` (audited via shell history).

Linters (`golangci-lint run`) and read-only diagnostics (`go vet`, `go fmt`) are allowed on host — they are read-only and don't fetch arbitrary code. The `bin/go` wrapper has an allowlist (vet, fmt, version, env, doc, list) that falls through unconditionally.

### Residual attack surface — kubeconfig mount

L1 note: The agent dev loop mounts the host kubeconfig read-only (`${KUBECONFIG:-$HOME/.kube/config}:/kubeconfig:ro`) so the agent can reach a local cluster during development. This is a **known residual attack surface** — a malicious Go dependency loaded at dev runtime could read cluster credentials inside the container layer (it cannot write to host, but it can exfiltrate). The supply-chain version-pinning policy in upstream repos mitigates the inbound risk, but does not eliminate it.

For dev work that doesn't require cluster access, run cluster-free:

```bash
KUBECONFIG=/dev/null make dev
```

The agent will start without kubeconfig and skip cluster-dependent code paths.

Verify (when alongside the kubesage workspace): `./ws verify-no-host-install`.

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

All build/test/dev verbs run inside containers per the
[Local Development Rule](#local-development-rule) above. Run
`direnv allow` once to enable the `./bin/` wrappers.

```bash
# One-time deps fetch (inside container)
make install

# Start the dev loop with HMR (air) — Ctrl+C to stop
make dev

# Build the binary inside container
make build

# Run tests inside container
make test

# Lint (host-side allowed — read-only, no fetch)
make lint

# Build production Docker image
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
