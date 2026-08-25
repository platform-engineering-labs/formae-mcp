package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// HostedOrigin is the only origin a hosted connection may address. It is a
// constant rather than configuration: a runtime setting that widens where
// credentials may be sent is an escape hatch that outlives its reason.
const HostedOrigin = "https://cloud.formae.ai"

// installationRE matches the canonical lowercase UUID text form. The constraint
// is syntactic and mirrors what the edge accepts as a routing key; it says
// nothing about UUID version or variant bits.
var installationRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Connection is where the MCP sends agent requests. It has exactly two arms, so
// a resolved configuration cannot be both classic and hosted, and a hosted one
// cannot exist without the installation it addresses.
type Connection interface {
	isConnection()
}

// Classic addresses a self-hosted agent.
type Classic struct {
	URL  string
	Port int
}

func (Classic) isConnection() {}

// Hosted addresses one installation behind the shared hosted endpoint.
type Hosted struct {
	Endpoint     string
	Installation string
}

func (Hosted) isConnection() {}

// ValidateHosted checks a hosted connection against a narrow grammar rather
// than comparing strings. "Exact match" without a stated grammar leaves
// implicit versus explicit :443, uppercase labels, percent-encoding and
// trailing dots undefined.
func ValidateHosted(h Hosted) error {
	if err := validateHostedEndpoint(h.Endpoint); err != nil {
		return err
	}
	if !installationRE.MatchString(h.Installation) {
		return fmt.Errorf("installation %q is not a canonical lowercase UUID", h.Installation)
	}
	return nil
}

func validateHostedEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("endpoint %q is not a valid URL: %w", endpoint, err)
	}
	switch {
	case u.Scheme != "https":
		return fmt.Errorf("endpoint %q must use https", endpoint)
	case u.User != nil:
		return fmt.Errorf("endpoint %q must not embed credentials", endpoint)
	case u.RawQuery != "":
		return fmt.Errorf("endpoint %q must not carry a query string", endpoint)
	case u.Fragment != "":
		return fmt.Errorf("endpoint %q must not carry a fragment", endpoint)
	case u.Path != "" && u.Path != "/":
		return fmt.Errorf("endpoint %q must not carry a path", endpoint)
	case u.Port() != "":
		return fmt.Errorf("endpoint %q must not name a port", endpoint)
	case u.Host != strings.ToLower(u.Host):
		return fmt.Errorf("endpoint %q must use a lowercase host", endpoint)
	}
	if "https://"+u.Host != HostedOrigin {
		return fmt.Errorf("endpoint %q is not the hosted endpoint %s", endpoint, HostedOrigin)
	}
	return nil
}
