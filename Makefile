BINARY    := jira-ai-creator
MODULE    := github.com/kengou/Jira-ticket-creator
GO        := go
GOFLAGS   :=
LDFLAGS   := -s -w

# Build metadata (overridable via environment or CLI)
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: all build test vet lint validate clean install fmt tidy help

## all: build the binary (default target)
all: build

## build: compile the binary
build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) .

## test: run all tests
test:
	$(GO) test $(GOFLAGS) -race ./...

## vet: run go vet
vet:
	$(GO) vet ./...

## lint: run staticcheck (install with: go install honnef.co/go/tools/cmd/staticcheck@latest)
lint: vet
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed — skipping (install: go install honnef.co/go/tools/cmd/staticcheck@latest)"

## validate: run vet, lint, and tests
validate: vet lint test

## fmt: format all Go source files
fmt:
	$(GO) fmt ./...

## tidy: tidy and verify module dependencies
tidy:
	$(GO) mod tidy
	$(GO) mod verify

## clean: remove build artifacts
clean:
	rm -f $(BINARY)

## install: install the binary to $GOPATH/bin
install:
	$(GO) install $(GOFLAGS) -ldflags "$(LDFLAGS)" .

## docker-build: build the Docker image
docker-build:
	docker build -t $(BINARY):$(VERSION) -t $(BINARY):latest .

## help: show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
