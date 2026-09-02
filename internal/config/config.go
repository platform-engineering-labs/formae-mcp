package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/platform-engineering-labs/formae-mcp/internal/profile"
)

const (
	defaultURL  = "http://localhost"
	defaultPort = "49684"
)

// deprecatedEnvWarnOnce ensures the FORMAE_AGENT_URL/PORT deprecation warning is
// emitted at most once per process, since AgentEndpoint runs per MCP tool call.
var deprecatedEnvWarnOnce sync.Once

// AgentEndpoint resolves the formae agent endpoint for an optional profile.
// Precedence: a profile always wins. An explicit requested profile is resolved
// first, else the active pointer; either resolving-but-unparseable is a hard
// error, never a silent fallback. Only when no profile is configured at all do
// the deprecated FORMAE_AGENT_URL/FORMAE_AGENT_PORT env vars apply, falling back
// to the localhost default for whichever endpoint field they leave unset.
func AgentEndpoint(profileName string) (url, port string, err error) {
	envURL := os.Getenv("FORMAE_AGENT_URL")
	envPort := os.Getenv("FORMAE_AGENT_PORT")
	if envURL != "" || envPort != "" {
		deprecatedEnvWarnOnce.Do(func() {
			slog.Warn("FORMAE_AGENT_URL/FORMAE_AGENT_PORT are deprecated in favour of formae profiles and will be removed. " +
				"When a profile is configured (active or per-call) it takes precedence and these variables are ignored; set up a profile instead.")
		})
	}

	switch {
	case profileName != "":
		url, port, err = endpointFromProfile(profileName)
	default:
		active, aerr := profile.ActiveProfile()
		if aerr == nil {
			url, port, err = endpointFromProfile(active)
		} else if !errors.Is(aerr, profile.ErrNotInitialized) {
			return "", "", aerr
		} else {
			// genuinely unconfigured: deprecated env fallback below
			url, port = envURL, envPort
		}
	}
	if err != nil {
		return "", "", err
	}

	// Fill absent endpoint fields from the localhost default (never from env,
	// which is only consulted when no profile is configured at all).
	if url == "" {
		url = defaultURL
	}
	if port == "" {
		port = defaultPort
	}
	return url, port, nil
}

// AgentCredentials resolves the HTTP basic credentials for an optional profile,
// following the same profile precedence as AgentEndpoint: an explicit profile
// if given, else the active pointer.
//
// Absent credentials are not an error. Most agents are reachable over the
// tailnet with no auth at all, so an empty result simply means "send no
// Authorization header". Only a profile that cannot be read is an error, and
// an environment with no profile configured yields nothing rather than failing.
func AgentCredentials(profileName string) (username, password string, err error) {
	name := profileName
	if name == "" {
		active, aerr := profile.ActiveProfile()
		if aerr != nil {
			if errors.Is(aerr, profile.ErrNotInitialized) {
				return "", "", nil
			}
			return "", "", aerr
		}
		name = active
	}

	path, err := profile.ProfilePath(name)
	if err != nil {
		return "", "", err
	}
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		return "", "", fmt.Errorf("profile %q not found: %w", name, rerr)
	}
	username, password = parseCliAuth(string(data))
	return username, password, nil
}

// endpointFromProfile reads a profile's PKL and extracts its cli.api endpoint.
// A profile that exists but yields neither url nor port is a hard error.
func endpointFromProfile(name string) (url, port string, err error) {
	path, err := profile.ProfilePath(name)
	if err != nil {
		return "", "", err
	}
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		return "", "", fmt.Errorf("profile %q not found: %w", name, rerr)
	}
	url, port = parseCliAPI(string(data))
	if url == "" && port == "" {
		return "", "", fmt.Errorf("profile %q has no resolvable cli.api endpoint", name)
	}
	return url, port, nil
}

var (
	urlPattern  = regexp.MustCompile(`url\s*=\s*"([^"]+)"`)
	portPattern = regexp.MustCompile(`port\s*=\s*(\d+)`)
)

// parseCliAPI extracts url and port from within the cli { api { ... } } block
// in a PKL config file. Uses simple brace-depth tracking.
func parseCliAPI(content string) (url, port string) {
	lines := strings.Split(content, "\n")

	inCli := false
	inAPI := false
	cliOpenCount := 0 // tracks cli block depth relative to its opening
	apiOpenCount := 0 // tracks api block depth relative to its opening

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Check for cli block entry (anywhere on the line, not just at start)
		if !inCli && strings.Contains(trimmed, "cli") && strings.Contains(trimmed, "{") {
			idx := strings.Index(trimmed, "cli")
			if idx >= 0 {
				before := idx > 0 && (trimmed[idx-1] >= 'a' && trimmed[idx-1] <= 'z' || trimmed[idx-1] >= 'A' && trimmed[idx-1] <= 'Z')
				after := idx+3 < len(trimmed) && (trimmed[idx+3] >= 'a' && trimmed[idx+3] <= 'z' || trimmed[idx+3] >= 'A' && trimmed[idx+3] <= 'Z')
				if !before && !after {
					inCli = true
					cliOpenCount = 0
				}
			}
		}

		// Check for api block entry (anywhere on the line, but only if we're in cli)
		if inCli && !inAPI && strings.Contains(trimmed, "api") && strings.Contains(trimmed, "{") {
			idx := strings.Index(trimmed, "api")
			if idx >= 0 {
				before := idx > 0 && (trimmed[idx-1] >= 'a' && trimmed[idx-1] <= 'z' || trimmed[idx-1] >= 'A' && trimmed[idx-1] <= 'Z')
				after := idx+3 < len(trimmed) && (trimmed[idx+3] >= 'a' && trimmed[idx+3] <= 'z' || trimmed[idx+3] >= 'A' && trimmed[idx+3] <= 'Z')
				if !before && !after {
					inAPI = true
					apiOpenCount = 0
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

		// Track braces for this line
		for _, ch := range trimmed {
			if ch == '{' {
				if inCli {
					cliOpenCount++
				}
				if inAPI {
					apiOpenCount++
				}
			} else if ch == '}' {
				if inAPI {
					apiOpenCount--
					if apiOpenCount == 0 {
						inAPI = false
					}
				}
				if inCli {
					cliOpenCount--
					if cliOpenCount == 0 {
						inCli = false
					}
				}
			}
		}
	}

	return url, port
}

var (
	usernamePattern = regexp.MustCompile(`username\s*=\s*"([^"]*)"`)
	passwordPattern = regexp.MustCompile(`password\s*=\s*"([^"]*)"`)
)

// opensBlock reports whether the line opens a block named name, as a whole
// identifier. Matches both `name {` and `name = new Something {`, which is how
// a profile writes `auth = new AuthBasic.CliConfig {`.
func opensBlock(line, name string) bool {
	idx := strings.Index(line, name)
	if idx < 0 || !strings.Contains(line[idx:], "{") {
		return false
	}
	isIdentChar := func(b byte) bool {
		return b == '_' ||
			(b >= 'a' && b <= 'z') ||
			(b >= 'A' && b <= 'Z') ||
			(b >= '0' && b <= '9')
	}
	if idx > 0 && isIdentChar(line[idx-1]) {
		return false
	}
	if end := idx + len(name); end < len(line) && isIdentChar(line[end]) {
		return false
	}
	return true
}

// parseCliAuth extracts HTTP basic-auth credentials from the cli block of a
// profile's PKL. It accepts them wherever they sit under `cli` — today's
// `cli.auth = new AuthBasic.CliConfig { ... }` and the `cli.connection`
// nesting the CLI is deprecating towards — because both spell the credentials
// the same way and only their position differs.
//
// Credentials outside `cli` are deliberately ignored: the agent block carries
// the server side of the same plugin, and sending that as client credentials
// would be wrong.
//
// Like parseCliAPI this is brace-depth text parsing, not a PKL evaluator, so a
// profile that computes its credentials rather than writing them literally
// yields nothing and the caller falls back to an unauthenticated client.
func parseCliAuth(content string) (username, password string) {
	inCli := false
	inAuth := false
	cliDepth := 0
	authDepth := 0

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		if !inCli && opensBlock(trimmed, "cli") {
			inCli = true
			cliDepth = 0
		}
		if inCli && !inAuth && opensBlock(trimmed, "auth") {
			inAuth = true
			authDepth = 0
		}

		if inCli && inAuth {
			if m := usernamePattern.FindStringSubmatch(trimmed); len(m) > 1 {
				username = m[1]
			}
			if m := passwordPattern.FindStringSubmatch(trimmed); len(m) > 1 {
				password = m[1]
			}
		}

		for _, ch := range trimmed {
			switch ch {
			case '{':
				if inCli {
					cliDepth++
				}
				if inAuth {
					authDepth++
				}
			case '}':
				if inAuth {
					authDepth--
					if authDepth == 0 {
						inAuth = false
					}
				}
				if inCli {
					cliDepth--
					if cliDepth == 0 {
						inCli = false
					}
				}
			}
		}
	}

	return username, password
}
