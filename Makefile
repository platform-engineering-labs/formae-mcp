.PHONY: build install test clean

BINARY_NAME := formae-mcp
VERSION ?= dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY_NAME) ./cmd/formae-mcp/

install:
	go install -ldflags "-X main.version=$(VERSION)" ./cmd/formae-mcp/

test:
	go test ./... -count=1 -timeout 30s

clean:
	rm -f $(BINARY_NAME)
