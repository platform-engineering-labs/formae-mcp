package config

import "testing"

func TestValidateHosted_AcceptsCanonicalEndpointAndInstallation(t *testing.T) {
	h := Hosted{
		Endpoint:     "https://cloud.formae.ai",
		Installation: "3f2b8c14-0000-4000-8000-000000000000",
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
				Installation: "3f2b8c14-0000-4000-8000-000000000000",
			})
			if err == nil {
				t.Fatalf("expected %s endpoint %q to be rejected", name, endpoint)
			}
		})
	}
}

func TestValidateHosted_RejectsBadInstallations(t *testing.T) {
	cases := map[string]string{
		"uppercase":   "3F2B8C14-0000-4000-8000-000000000000",
		"braces":      "{3f2b8c14-0000-4000-8000-000000000000}",
		"too short":   "3f2b8c14-0000-4000-8000-00000000000",
		"not a uuid":  "default",
		"empty":       "",
		"with spaces": "3f2b8c14-0000-4000-8000-000000000000 ",
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
