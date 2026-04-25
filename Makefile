.PHONY: build test lint docker clean dev install dev-up dev-down

BINARY=kubesage-agent

# ---------------------------------------------------------------------------
# Phase 20.5 — Containerized dev/build/test/install.
# Local Development Rule: see workspace kubesage/CLAUDE.md > Local Development Rule
# C1 fix: each verb target has its recipe DIRECTLY in the same target body.
# ---------------------------------------------------------------------------

## install: go mod download inside container.
install:
	KUBESAGE_ALLOW_HOST=1 docker compose --profile dev run --rm agent go mod download

## dev: Bring up agent with HMR via docker compose watch.
## C1 fix: recipe DIRECTLY under `dev:` (calls dev-up as prereq, runs watch in own body).
dev: dev-up
	docker compose --profile dev watch

dev-up:
	docker compose --profile dev up -d

dev-down:
	docker compose --profile dev down

## build: Build the agent binary inside container.
build:
	KUBESAGE_ALLOW_HOST=1 docker compose --profile dev run --rm agent go build -o /tmp/$(BINARY) ./cmd/agent

## test: Run tests inside container.
test:
	KUBESAGE_ALLOW_HOST=1 docker compose --profile dev run --rm agent go test -race ./...

## lint: Run golangci-lint (host-side allowed per Local Development Rule).
lint:
	golangci-lint run ./...

## docker: Build production image.
docker:
	docker build -t kubesage-agent:dev .

## clean: Remove build artifacts.
clean:
	rm -f $(BINARY)
