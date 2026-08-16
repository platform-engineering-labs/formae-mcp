package config

import "testing"

func TestValidateHosted_AcceptsCanonicalEndpointAndInstallation(t *testing.T) {
	h := Hosted{
		Endpoint:     "https://cloud.formae.ai",
		Installation: "3HzFPXfPDGhwLJJVtaHbmFs6vLa",
	}
	if err := ValidateHosted(h); err != nil {
		t.Fatalf("expected valid hosted connection, got error: %v", err)
	}
}

func TestValidateHosted_RejectsBadEndpoints(t *testing.T) {
	cases := map[string]string{
		"plain http":     "http://cloud.formae.ai",
		"explicit port":  "https://cloud.formae.ai:8443",
		"userinfo":       "https://user:pw@cloud.formae.ai",
		"path":           "https://cloud.formae.ai/api",
		"query":          "https://cloud.formae.ai?a=b",
		"fragment":       "https://cloud.formae.ai#f",
		"uppercase host": "https://CLOUD.formae.ai",
		"trailing dot":   "https://cloud.formae.ai.",
		"other host":     "https://evil.example",
		"empty":          "",
	}
	for name, endpoint := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateHosted(Hosted{
				Endpoint:     endpoint,
				Installation: "3HzFPXfPDGhwLJJVtaHbmFs6vLa",
			})
			if err == nil {
				t.Fatalf("expected %s endpoint %q to be rejected", name, endpoint)
			}
		})
	}
}

func TestValidateHosted_RejectsBadInstallations(t *testing.T) {
	cases := map[string]string{
		// The format installations used to carry. Nothing mints one now, so a
		// profile naming one addresses an installation that cannot exist.
		"the retired uuid form": "3f2b8c14-0000-4000-8000-000000000000",
		"braces":                "{3HzFPXfPDGhwLJJVtaHbmFs6vLa}",
		"one short":             "3HzFPXfPDGhwLJJVtaHbmFs6vL",
		"one long":              "3HzFPXfPDGhwLJJVtaHbmFs6vLaa",
		"a hyphen":              "3HzFPXfPDGhwLJJVtaHbmFs6v-a",
		"an underscore":         "3HzFPXfPDGhwLJJVtaHbmFs6v_a",
		"not an installation":   "default",
		"empty":                 "",
		"trailing space":        "3HzFPXfPDGhwLJJVtaHbmFs6vL ",
		"a newline":             "3HzFPXfPDGhwLJJVtaHbmFs6vLa\n",
	}
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateHosted(Hosted{Endpoint: HostedOrigin, Installation: id})
			if err == nil {
				t.Fatalf("expected %s installation %q to be rejected", name, id)
			}
		})
	}
}

// The check is the routing key's grammar, not a decode. 27 base62 digits span a
// wider range than the 160 bits a KSUID encodes, so a few well-formed strings
// would fail a KSUID parser. Refusing them here would make this client stricter
// than the edge that does the routing, which validates the same grammar: we
// would refuse an identifier the router accepts and gain nothing, because
// nothing mints one that cannot be decoded. Pinned so the limit is a decision.
func TestValidateHosted_ChecksTheRoutingGrammarNotADecode(t *testing.T) {
	err := ValidateHosted(Hosted{
		Endpoint:     HostedOrigin,
		Installation: "zzzzzzzzzzzzzzzzzzzzzzzzzzz",
	})
	if err != nil {
		t.Fatalf("a well-formed identifier must be accepted without decoding it: %v", err)
	}
}
