.PHONY: build install test clean pkg-bin

BINARY_NAME := formae-mcp

# The latest tag, possibly carrying a `-channel` suffix (e.g., 0.85.0-dev.2).
RAW_VERSION := $(shell git describe --tags --abbrev=0 --match "[0-9]*" --match "v[0-9]*" 2>/dev/null || echo "0.0.0")
# Canonical semver — everything before the first `-`.
VERSION ?= $(shell echo "$(RAW_VERSION)" | cut -d'-' -f1)
LDFLAGS := -ldflags "-X github.com/platform-engineering-labs/formae-mcp/internal/version.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/formae-mcp/

install:
	go install $(LDFLAGS) ./cmd/formae-mcp/

test:
	go test ./... -count=1 -timeout 30s

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/
	rm -f version.semver

pkg-bin: build
	echo '$(VERSION)' > ./version.semver
	mkdir -p ./dist/pel/bin
	cp -Rp ./$(BINARY_NAME) ./dist/pel/bin
