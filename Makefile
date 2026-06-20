.PHONY: build install test clean

BINARY_NAME := formae-mcp

# Build version, injected into main.version. Falls back to "dev" so the
# Go-level default is never overwritten with an empty string.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null)
ifeq ($(strip $(VERSION)),)
	VERSION := dev
endif
LDFLAGS := -X main.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/formae-mcp/

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/formae-mcp/

test:
	go test ./... -count=1 -timeout 30s

clean:
	rm -f $(BINARY_NAME)
