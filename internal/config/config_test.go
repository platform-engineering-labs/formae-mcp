package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCliAPI_WithValues(t *testing.T) {
	content := `amends "formae:/Config.pkl"

agent {
    server {
        hostname = "localhost"
        port = 49684
    }
}

cli {
    api {
        url = "http://my-agent.example.com"
        port = 9999
    }
    disableUsageReporting = true
}
`
	url, port := parseCliAPI(content)
	if url != "http://my-agent.example.com" {
		t.Errorf("expected url 'http://my-agent.example.com', got '%s'", url)
	}
	if port != "9999" {
		t.Errorf("expected port '9999', got '%s'", port)
	}
}

func TestParseCliAPI_NoCliBlock(t *testing.T) {
	content := `amends "formae:/Config.pkl"

agent {
    server {
        hostname = "localhost"
        port = 49684
    }
}
`
	url, port := parseCliAPI(content)
	if url != "" {
		t.Errorf("expected empty url, got '%s'", url)
	}
	if port != "" {
		t.Errorf("expected empty port, got '%s'", port)
	}
}

func TestParseCliAPI_CliWithoutAPI(t *testing.T) {
	content := `amends "formae:/Config.pkl"

cli {
    disableUsageReporting = true
}
`
	url, port := parseCliAPI(content)
	if url != "" {
		t.Errorf("expected empty url, got '%s'", url)
	}
	if port != "" {
		t.Errorf("expected empty port, got '%s'", port)
	}
}

func TestParseCliAPI_DoNotPickUpAgentPort(t *testing.T) {
	content := `amends "formae:/Config.pkl"

agent {
    server {
        hostname = "my-host"
        port = 12345
    }
}

cli {
    disableUsageReporting = true
}
`
	url, port := parseCliAPI(content)
	if url != "" {
		t.Errorf("expected empty url, got '%s'", url)
	}
	if port != "" {
		t.Errorf("expected empty port (should not pick up agent port), got '%s'", port)
	}
}

func TestParseCliAPI_WithComments(t *testing.T) {
	content := `amends "formae:/Config.pkl"

cli {
    api {
        // url = "http://commented-out"
        url = "http://actual"
        port = 8080
    }
}
`
	url, port := parseCliAPI(content)
	if url != "http://actual" {
		t.Errorf("expected url 'http://actual', got '%s'", url)
	}
	if port != "8080" {
		t.Errorf("expected port '8080', got '%s'", port)
	}
}

func TestParseCliAPI_OnlyURL(t *testing.T) {
	content := `amends "formae:/Config.pkl"

cli {
    api {
        url = "http://custom-host"
    }
}
`
	url, port := parseCliAPI(content)
	if url != "http://custom-host" {
		t.Errorf("expected url 'http://custom-host', got '%s'", url)
	}
	if port != "" {
		t.Errorf("expected empty port, got '%s'", port)
	}
}

func writeProfile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, "profiles", name+".pkl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sampleProfile = `amends "formae:/Config.pkl"
cli {
    api {
        url = "http://agent.example.com"
        port = 9000
    }
}
`

func TestAgentEndpoint_NoProfileBothEnvFallback(t *testing.T) {
	t.Setenv("FORMAE_CONFIG_DIR", t.TempDir()) // empty, no active
	t.Setenv("FORMAE_AGENT_URL", "http://env-host")
	t.Setenv("FORMAE_AGENT_PORT", "1234")
	url, port, err := AgentEndpoint("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if url != "http://env-host" || port != "1234" {
		t.Errorf("got %s:%s", url, port)
	}
}

func TestAgentEndpoint_NoProfileOnlyURLFallback(t *testing.T) {
	t.Setenv("FORMAE_CONFIG_DIR", t.TempDir()) // empty, no active
	t.Setenv("FORMAE_AGENT_URL", "http://env-host")
	t.Setenv("FORMAE_AGENT_PORT", "")
	url, port, err := AgentEndpoint("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if url != "http://env-host" || port != "49684" {
		t.Errorf("expected env url + default port, got %s:%s", url, port)
	}
}

func TestAgentEndpoint_NoProfileOnlyPortFallback(t *testing.T) {
	t.Setenv("FORMAE_CONFIG_DIR", t.TempDir()) // empty, no active
	t.Setenv("FORMAE_AGENT_URL", "")
	t.Setenv("FORMAE_AGENT_PORT", "1234")
	url, port, err := AgentEndpoint("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if url != "http://localhost" || port != "1234" {
		t.Errorf("expected default url + env port, got %s:%s", url, port)
	}
}

func TestAgentEndpoint_ActiveProfileBeatsEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	t.Setenv("FORMAE_AGENT_URL", "http://env-host")
	t.Setenv("FORMAE_AGENT_PORT", "1234")
	writeProfile(t, dir, "prod", sampleProfile)
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte("prod\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	url, port, err := AgentEndpoint("")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://agent.example.com" || port != "9000" {
		t.Errorf("profile must win over env, got %s:%s", url, port)
	}
}

func TestAgentEndpoint_ExplicitProfileBeatsActiveAndEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	t.Setenv("FORMAE_AGENT_URL", "http://env-host")
	t.Setenv("FORMAE_AGENT_PORT", "1234")
	writeProfile(t, dir, "a", `amends "formae:/Config.pkl"
cli {
    api {
        url = "http://profile-a"
        port = 1111
    }
}
`)
	writeProfile(t, dir, "b", sampleProfile)
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	url, port, err := AgentEndpoint("a")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://profile-a" || port != "1111" {
		t.Errorf("explicit profile must win over active and env, got %s:%s", url, port)
	}
}

func TestAgentEndpoint_EnvDoesNotRescueStaleActive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	t.Setenv("FORMAE_AGENT_URL", "http://env-host")
	t.Setenv("FORMAE_AGENT_PORT", "1234")
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte("ghost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := AgentEndpoint("")
	if err == nil {
		t.Fatal("expected hard error for stale active pointer; env must not rescue it")
	}
}

func TestAgentEndpoint_EnvDoesNotRescueMissingExplicit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	t.Setenv("FORMAE_AGENT_URL", "http://env-host")
	t.Setenv("FORMAE_AGENT_PORT", "1234")
	_, _, err := AgentEndpoint("ghost")
	if err == nil {
		t.Fatal("expected hard error for missing requested profile; env must not rescue it")
	}
}

func TestAgentEndpoint_ExplicitProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	t.Setenv("FORMAE_AGENT_URL", "")
	t.Setenv("FORMAE_AGENT_PORT", "")
	writeProfile(t, dir, "prod", sampleProfile)
	url, port, err := AgentEndpoint("prod")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://agent.example.com" || port != "9000" {
		t.Errorf("got %s:%s", url, port)
	}
}

func TestAgentEndpoint_ProfileMissingPortDefaultsLocalhost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	t.Setenv("FORMAE_AGENT_URL", "http://env-host")
	t.Setenv("FORMAE_AGENT_PORT", "1234")
	writeProfile(t, dir, "prod", `amends "formae:/Config.pkl"
cli {
    api {
        url = "http://url-only"
    }
}
`)
	url, port, err := AgentEndpoint("prod")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://url-only" || port != "49684" {
		t.Errorf("expected profile url + default port (never env), got %s:%s", url, port)
	}
}

func TestAgentEndpoint_RequestedMissingProfileHardErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	t.Setenv("FORMAE_AGENT_URL", "")
	t.Setenv("FORMAE_AGENT_PORT", "")
	_, _, err := AgentEndpoint("ghost")
	if err == nil {
		t.Fatal("expected hard error for missing requested profile, got nil")
	}
	if strings.Contains(err.Error(), "localhost") {
		t.Errorf("must not fall back to localhost: %v", err)
	}
}

func TestAgentEndpoint_ActivePointer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	t.Setenv("FORMAE_AGENT_URL", "")
	t.Setenv("FORMAE_AGENT_PORT", "")
	writeProfile(t, dir, "prod", sampleProfile)
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte("prod\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	url, port, err := AgentEndpoint("")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://agent.example.com" || port != "9000" {
		t.Errorf("got %s:%s", url, port)
	}
}

func TestAgentEndpoint_StaleActiveHardErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	t.Setenv("FORMAE_AGENT_URL", "")
	t.Setenv("FORMAE_AGENT_PORT", "")
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte("ghost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := AgentEndpoint("")
	if err == nil {
		t.Fatal("expected hard error for stale active pointer")
	}
}

func TestAgentEndpoint_UnconfiguredDefaultsLocalhost(t *testing.T) {
	t.Setenv("FORMAE_CONFIG_DIR", t.TempDir())
	t.Setenv("FORMAE_AGENT_URL", "")
	t.Setenv("FORMAE_AGENT_PORT", "")
	url, port, err := AgentEndpoint("")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://localhost" || port != "49684" {
		t.Errorf("got %s:%s", url, port)
	}
}

func TestParseCliAPI_CompactBlock(t *testing.T) {
	content := `amends "formae:/Config.pkl"
cli { api { url = "http://compact" port = 8080 } }
`
	url, port := parseCliAPI(content)
	if url != "http://compact" || port != "8080" {
		t.Errorf("compact block not parsed: url=%q port=%q", url, port)
	}
}

func TestParseCliAPI_StateResetAfterCliBlock(t *testing.T) {
	// Regression test: after a complete cli { api { ... } } block the state must
	// be fully reset so a later agent { api { port = N } } block is not
	// mis-parsed as cli.api values.
	content := `amends "formae:/Config.pkl"
cli { api { url = "http://right" port = 8080 } }

agent {
    api {
        port = 12345
    }
}
`
	url, port := parseCliAPI(content)
	if url != "http://right" {
		t.Errorf("expected url 'http://right', got %q", url)
	}
	if port != "8080" {
		t.Errorf("expected port '8080' (not agent.api.port 12345), got %q", port)
	}
}
