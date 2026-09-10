// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package codebase

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
)

// IdentityForConnection consumes an already resolved connection. It deliberately
// accepts no profile name, credential or user identity.
func IdentityForConnection(conn config.Connection) (Identity, error) {
	switch c := conn.(type) {
	case config.Hosted:
		return normalizeIdentity(Identity{Kind: "hosted", Endpoint: c.Endpoint, Installation: c.Installation})
	case config.Classic:
		endpoint := c.URL
		if c.Port != 0 {
			endpoint = fmt.Sprintf("%s:%d", c.URL, c.Port)
		}
		return normalizeIdentity(Identity{Kind: "classic", Endpoint: endpoint})
	default:
		return Identity{}, errors.New("no resolved installation connection")
	}
}

func normalizeIdentity(identity Identity) (Identity, error) {
	switch identity.Kind {
	case "hosted":
		if err := config.ValidateHosted(config.Hosted{Endpoint: identity.Endpoint, Installation: identity.Installation}); err != nil {
			return Identity{}, err
		}
		identity.Endpoint = strings.TrimSuffix(identity.Endpoint, "/")
	case "classic":
		u, err := url.Parse(identity.Endpoint)
		if err != nil {
			return Identity{}, errors.New("invalid classic endpoint")
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || identity.Installation != "" {
			return Identity{}, errors.New("classic identity requires a credential-free HTTP endpoint")
		}
		// Preserve the routed endpoint, including any path or explicit port. This
		// fallback must not claim two different routed endpoints are one agent.
	default:
		return Identity{}, errors.New("unknown installation identity kind")
	}
	return identity, nil
}
