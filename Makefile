BINARY := talea
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/talea/talea/internal/version.Version=$(VERSION) \
           -X github.com/talea/talea/internal/version.Commit=$(COMMIT) \
           -X github.com/talea/talea/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: build test lint vet tidy clean release release-cross

build:
	go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/talea

# 交叉编译多平台二进制到 dist/
release-cross:
	@mkdir -p dist
	GOOS=linux   GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-linux-amd64       ./cmd/talea
	GOOS=linux   GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-linux-arm64       ./cmd/talea
	GOOS=darwin  GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-darwin-amd64      ./cmd/talea
	GOOS=darwin  GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-darwin-arm64      ./cmd/talea
	GOOS=windows GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-windows-amd64.exe ./cmd/talea
	GOOS=windows GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-windows-arm64.exe ./cmd/talea
	@echo "Cross-built binaries in dist/"

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
