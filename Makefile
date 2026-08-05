BINARY := talea
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/talea/talea/internal/version.Version=$(VERSION) \
           -X github.com/talea/talea/internal/version.Commit=$(COMMIT) \
           -X github.com/talea/talea/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: build test lint vet tidy clean release

build:
	go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/talea

test:
	go test ./...

lint:
	golangci-lint run

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf bin dist

release:
	goreleaser release --clean
