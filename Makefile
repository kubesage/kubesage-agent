.PHONY: build test lint docker clean

BINARY=kubesage-agent

build:
	go build -o $(BINARY) ./cmd/agent/

test:
	go test -race ./...

lint:
	golangci-lint run ./...

docker:
	docker build -t kubesage-agent:dev .

clean:
	rm -f $(BINARY)
