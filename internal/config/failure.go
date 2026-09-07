package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
)

// failureSchemaVersion is the envelope shape this build understands. It is
// checked before any other field, exactly as the success document's is.
const failureSchemaVersion = 1

// diagnose is the invitation attached to every declared failure. It is the same
// command, run by hand, and its human output is deliberately not redacted the
// way this consumer is: a person looking at their own profile in their own
// terminal is not the exposure we are closing.
const diagnose = "run `formae connection resolve` to see why"

// declaredCodes is the closed namespace the producer promises. A code outside
// it is a protocol mismatch, not a message to pass along.
var declaredCodes = map[string]bool{
	"ambiguous_profile": true,
	"auth_failed":       true,
	"untrusted_issuer":  true,
	"no_connection":     true,
	"internal":          true,
}

// declaredPluginCodes is what an auth plugin may report.
//
// This is validated rather than trusted, unlike the top-level code, because the
// producer does not close it: the plugin's error code is a bare string alias
// copied through without a membership check, so `details.pluginCode` is an open
// channel wearing a closed channel's name. A plugin that put a token there
// would otherwise route it straight into a tool result.
var declaredPluginCodes = map[string]bool{
	"unsupported":        true,
	"not_logged_in":      true,
	"session_expired":    true,
	"issuer_unreachable": true,
}

// unrecognisedPluginCode replaces a plugin code we did not declare. It says a
// code was present and withheld, which is all a reader can safely be told.
const unrecognisedPluginCode = "unrecognised"

// ResolveError is a failure the CLI declared. It carries the code so a caller
// can branch on it, and never the producer's free-text message.
type ResolveError struct {
	Code string
	// PluginCode is the auth plugin's own code for an auth_failed, validated
	// against the declared set. Empty when absent or unrecognised.
	PluginCode string
}

func (e *ResolveError) Error() string {
	switch e.Code {
	case "auth_failed":
		if e.PluginCode != "" {
			return fmt.Sprintf("formae could not obtain a credential for this profile (%s); %s",
				e.PluginCode, diagnose)
		}
		return fmt.Sprintf("formae could not obtain a credential for this profile; %s", diagnose)
	case "untrusted_issuer":
		return fmt.Sprintf(
			"this profile's hosted connection names an issuer this build will not authenticate against; %s",
			diagnose)
	case "no_connection":
		return fmt.Sprintf("this profile resolves no connection formae can use; %s", diagnose)
	default:
		return fmt.Sprintf("formae could not resolve the connection; %s", diagnose)
	}
}

// AmbiguousProfileError is the CLI refusing to guess which installation was
// meant. Its message is the instruction the caller acts on, so a model reading
// it can retry with the argument rather than needing to be told separately.
type AmbiguousProfileError struct {
	Candidates []string
	Active     string
}

func (e *AmbiguousProfileError) Error() string {
	listed := make([]string, 0, len(e.Candidates))
	for _, c := range e.Candidates {
		if c == e.Active {
			c += " (active)"
		}
		listed = append(listed, c)
	}
	return fmt.Sprintf(
		"more than one profile exists and none was named, so formae cannot tell which "+
			"installation you meant. Pass the profile argument on this call: %s",
		strings.Join(listed, ", "))
}

// failureView is the envelope the producer emits on stdout when it fails.
type failureView struct {
	SchemaVersion *int   `json:"schemaVersion"`
	Code          string `json:"code"`
	Details       struct {
		Candidates []string `json:"candidates"`
		Active     string   `json:"active"`
		PluginCode string   `json:"pluginCode"`
	} `json:"details"`
	// Message is decoded so it is visibly accounted for, and deliberately never
	// read: the producer builds it from an auth plugin's error string or an
	// arbitrary err.Error(), and a Pkl failure quotes profile source lines,
	// which for a classic profile can mean an inline password.
	Message string `json:"message"`
}

// decodeFailure turns a non-zero exit into a typed error.
//
// exitStatus names the failure when the envelope cannot be read at all, which
// is a supported path rather than a defensive one: argv the command cannot
// parse fails before the flags that say how to render a failure exist, so it
// exits non-zero with no envelope. The raw bytes never reach the error.
func decodeFailure(stdout []byte, exitStatus int) error {
	unreadable := fmt.Errorf("formae could not resolve the connection (exit %d); %s",
		exitStatus, diagnose)

	var v failureView
	dec := json.NewDecoder(bytes.NewReader(stdout))
	if err := dec.Decode(&v); err != nil {
		return unreadable
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return unreadable
	}
	if v.SchemaVersion == nil || *v.SchemaVersion != failureSchemaVersion {
		return unreadable
	}
	if !declaredCodes[v.Code] {
		return unreadable
	}

	if v.Code == "ambiguous_profile" {
		return &AmbiguousProfileError{
			Candidates: v.Details.Candidates,
			Active:     v.Details.Active,
		}
	}

	pluginCode := v.Details.PluginCode
	if pluginCode != "" && !declaredPluginCodes[pluginCode] {
		pluginCode = unrecognisedPluginCode
	}
	return &ResolveError{Code: v.Code, PluginCode: pluginCode}
}

// Resolved is one profile evaluation: the effective profile name the CLI
// reported, the connection it resolved, and the credential that reaches it.
type Resolved struct {
	Profile string
	Conn    Connection
	// Credential is the zero value for classic: the MCP sends a self-hosted
	// agent none, and that is a non-goal rather than an omission.
	Credential secret.Value
}
