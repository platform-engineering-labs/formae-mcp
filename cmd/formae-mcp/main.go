package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/server"
)

// version is the build version, injectable via -ldflags "-X main.version=...".
var version = "dev"

// run handles flags that short-circuit server startup. If any arg is exactly
// --version or -version, it writes the version to stdout and returns true; the
// caller should then exit without starting the server. Other args are ignored.
func run(args []string, stdout io.Writer) (handled bool) {
	for _, arg := range args {
		if arg == "--version" || arg == "-version" {
			fmt.Fprintln(stdout, version)
			return true
		}
	}
	return false
}

func main() {
	if run(os.Args[1:], os.Stdout) {
		return
	}

	agentURL, agentPort := config.AgentEndpoint()
	endpoint := fmt.Sprintf("%s:%s", agentURL, agentPort)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	s := server.New(endpoint)
	if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
