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

// handleVersion prints the version and reports true if args request it via
// --version or -version; otherwise it writes nothing and returns false.
func handleVersion(args []string, out io.Writer) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-version" {
			fmt.Fprintln(out, version)
			return true
		}
	}
	return false
}

func main() {
	if handleVersion(os.Args[1:], os.Stdout) {
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
