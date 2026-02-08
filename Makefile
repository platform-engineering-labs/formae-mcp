.PHONY: build install test clean

BINARY_NAME := formae-mcp

build:
	go build -o $(BINARY_NAME) ./cmd/formae-mcp/

install:
	go install ./cmd/formae-mcp/

test:
	go test ./... -count=1 -timeout 30s

clean:
	rm -f $(BINARY_NAME)
