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
	"github.com/platform-engineering-labs/formae-mcp/internal/version"
)

// usage is the help message printed by tryHelp.
const usage = `formae-mcp - Model Context Protocol server for formae

Usage:
  formae-mcp [flags]

Flags:
  -h, --help       Show this help message and exit
      --version    Print the version and exit
`

// tryHelp handles the --help flag. If args contains an exact --help (-help or
// -h) token, it writes the usage message to stdout and returns true; otherwise
// it writes nothing and returns false.
func tryHelp(args []string, stdout io.Writer) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-help" || arg == "-h" {
			fmt.Fprint(stdout, usage)
			return true
		}
	}
	return false
}

// tryVersion handles the --version flag. If args contains an exact --version
// (or -version) token, it writes version followed by a newline to stdout and
// returns true; otherwise it writes nothing and returns false.
func tryVersion(args []string, version string, stdout io.Writer) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-version" {
			fmt.Fprintln(stdout, version)
			return true
		}
	}
	return false
}

func main() {
	if tryHelp(os.Args[1:], os.Stdout) {
		return
	}

	if tryVersion(os.Args[1:], version.String(), os.Stdout) {
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
