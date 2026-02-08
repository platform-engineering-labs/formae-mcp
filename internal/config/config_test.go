package config

import "testing"

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
