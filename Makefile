.PHONY: build install test clean

BINARY_NAME := formae-mcp
VERSION ?= dev
LDFLAGS := -ldflags "-X github.com/platform-engineering-labs/formae-mcp/internal/version.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/formae-mcp/

install:
	go install $(LDFLAGS) ./cmd/formae-mcp/

test:
	go test ./... -count=1 -timeout 30s

clean:
	rm -f $(BINARY_NAME)
