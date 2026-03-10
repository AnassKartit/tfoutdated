BINARY := tfoutdated
MCP_BINARY := tfoutdated-mcp
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/anasskartit/tfoutdated/cmd.version=$(VERSION)"
MCP_LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build build-mcp build-all test lint clean install

build:
	go build $(LDFLAGS) -o $(BINARY) .

build-mcp:
	go build $(MCP_LDFLAGS) -o $(MCP_BINARY) ./cmd/tfoutdated-mcp

build-all: build build-mcp

install:
	go install $(LDFLAGS) .

test:
	go test ./... -v -race

lint:
	go vet ./...
	golangci-lint run ./...

clean:
	rm -f $(BINARY) $(MCP_BINARY)
	rm -rf dist/

release-snapshot:
	goreleaser release --snapshot --clean

fmt:
	gofmt -s -w .

.DEFAULT_GOAL := build
