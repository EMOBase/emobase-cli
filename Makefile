BINARY  := emobase
PREFIX  ?= /usr/local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/EMOBase/emobase-cli/internal/cli.version=$(VERSION)

GOOS   ?= $(shell go env GOHOSTOS)
GOARCH ?= $(shell go env GOHOSTARCH)

.PHONY: all build cross build-linux install install-user test vet fmt tidy clean preview preview-case preview-all

all: build

# Host build: dist/emobase
build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o dist/$(BINARY) ./cmd/emobase

# Cross build: dist/emobase-<os>-<arch>, for example
#   make cross GOOS=linux GOARCH=amd64
cross:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 \
		go build -trimpath -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-$(GOOS)-$(GOARCH) ./cmd/emobase

# Both Linux architectures used by servers.
build-linux:
	$(MAKE) cross GOOS=linux GOARCH=amd64
	$(MAKE) cross GOOS=linux GOARCH=arm64
	@ls -lh dist/$(BINARY)-linux-*

# System install (may need sudo, or pass PREFIX=/opt/homebrew on Apple Silicon).
install:
	@mkdir -p $(PREFIX)/bin
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(PREFIX)/bin/$(BINARY) ./cmd/emobase

# No-sudo install into ~/.local/bin; add that directory to PATH.
install-user:
	$(MAKE) install PREFIX=$(HOME)/.local

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

# Development-only UI case browser (not shipped in the release binary).
preview:
	go run ./cmd/preview list

preview-case:
	@test -n "$(CASE)" || (echo "usage: make preview-case CASE=doctor-findings" && exit 1)
	go run ./cmd/preview $(CASE)

preview-all:
	go run ./cmd/preview --all

clean:
	rm -rf dist
