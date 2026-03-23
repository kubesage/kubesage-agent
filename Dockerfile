# Multi-stage build for KubeSage cluster agent
# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /kubesage-agent ./cmd/agent/

# Runtime stage
FROM alpine:3.21
RUN apk --no-cache add ca-certificates
COPY --from=builder /kubesage-agent /usr/local/bin/kubesage-agent
USER 65534:65534
ENTRYPOINT ["/usr/local/bin/kubesage-agent"]
