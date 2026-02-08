package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	configDir      = ".config/formae"
	configFileName = "formae.conf.pkl"
	defaultURL     = "http://localhost"
	defaultPort    = "49684"
)

// AgentEndpoint resolves the formae agent endpoint using this precedence:
//  1. Environment variables (FORMAE_AGENT_URL, FORMAE_AGENT_PORT)
//  2. Formae config file (~/.config/formae/formae.conf.pkl)
//  3. Hardcoded defaults (http://localhost:49684)
func AgentEndpoint() (url, port string) {
	url = defaultURL
	port = defaultPort

	// Try to read from config file
	if cfgURL, cfgPort, ok := readFromConfig(); ok {
		if cfgURL != "" {
			url = cfgURL
		}
		if cfgPort != "" {
			port = cfgPort
		}
	}

	// Environment variables take highest precedence
	if envURL := os.Getenv("FORMAE_AGENT_URL"); envURL != "" {
		url = envURL
	}
	if envPort := os.Getenv("FORMAE_AGENT_PORT"); envPort != "" {
		port = envPort
	}

	return url, port
}

var (
	urlPattern  = regexp.MustCompile(`url\s*=\s*"([^"]+)"`)
	portPattern = regexp.MustCompile(`port\s*=\s*(\d+)`)
)

// readFromConfig reads the formae CLI config file and extracts
// the api.url and api.port from the cli block.
func readFromConfig() (url, port string, ok bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", false
	}

	data, err := os.ReadFile(filepath.Join(home, configDir, configFileName))
	if err != nil {
		return "", "", false
	}

	content := string(data)
	url, port = parseCliAPI(content)
	return url, port, true
}

// parseCliAPI extracts url and port from within the cli { api { ... } } block
// in a PKL config file. Uses simple brace-depth tracking.
func parseCliAPI(content string) (url, port string) {
	lines := strings.Split(content, "\n")

	inCli := false
	inAPI := false
	depth := 0
	cliDepth := 0
	apiDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Track block entry
		if !inCli && strings.HasPrefix(trimmed, "cli") && strings.Contains(trimmed, "{") {
			inCli = true
			cliDepth = depth
			depth++
			continue
		}

		if inCli && !inAPI && strings.HasPrefix(trimmed, "api") && strings.Contains(trimmed, "{") {
			inAPI = true
			apiDepth = depth
			depth++
			continue
		}

		// Track braces
		for _, ch := range trimmed {
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if inAPI && depth == apiDepth {
					inAPI = false
				}
				if inCli && depth == cliDepth {
					inCli = false
				}
			}
		}

		// Extract values only when inside cli.api block
		if inCli && inAPI {
			if m := urlPattern.FindStringSubmatch(trimmed); len(m) > 1 {
				url = m[1]
			}
			if m := portPattern.FindStringSubmatch(trimmed); len(m) > 1 {
				port = m[1]
			}
		}
	}

	return url, port
}
