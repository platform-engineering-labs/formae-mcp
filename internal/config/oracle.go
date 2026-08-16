package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"

	"github.com/platform-engineering-labs/formae-mcp/internal/featuregate"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
)

// schemaVersion is the machine-view major version this build understands. It is
// checked before any other field: an unrecognised value is an error, never a
// guess at what the producer meant.
const schemaVersion = 1

// maxOracleOutput bounds the machine view. One overall bound, not per-field
// limits: the reachable failure is a runaway producer, not a large field.
const maxOracleOutput = 4 << 20

// oracleTimeout applies when the caller supplies no deadline of its own.
const oracleTimeout = 10 * time.Second

var errOutputTooLarge = errors.New("formae produced more output than expected")

// resolveVia runs the CLI as the configuration oracle and decodes its machine
// view. Stdout and stderr are captured separately and drained concurrently:
// combined capture would put subprocess output into errors, and reading two
// pipes serially can deadlock when the child fills the one not being read.
//
// One command produces both the connection and the credential. Two
// independently timed reads could not: between them the active pointer can
// move, the profile can be rewritten, or the auth block can change, and the
// request would carry an endpoint from one revision with a credential from
// another — for hosted, one installation's endpoint with another's credential.
func resolveVia(ctx context.Context, bin, profileName string, forceRefresh bool) (Resolved, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, oracleTimeout)
		defer cancel()
	}

	// --profile is a local flag on `resolve`, registered by AddConfigFlags, so
	// it follows the subcommand. This is the opposite of `profile show`, where
	// the name is positional and no such flag exists.
	args := []string{"connection", "resolve"}
	if profileName != "" {
		args = append(args, "--profile", profileName)
	}
	args = append(args, "--output-consumer", "machine", "--output-schema", "json")
	if forceRefresh {
		args = append(args, "--force-refresh")
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	// Own process group, so a deadline kills the CLI and anything it spawned.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	kill := func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	cmd.Cancel = func() error { kill(); return nil }

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Resolved{}, fmt.Errorf("resolving configuration: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Resolved{}, fmt.Errorf("resolving configuration: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return Resolved{}, fmt.Errorf("running %s: %w", bin, err)
	}

	// Stderr carries warnings and is never part of the parsed value. Drained
	// and discarded so the child cannot block on a full pipe.
	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	type readResult struct {
		data []byte
		err  error
	}
	outCh := make(chan readResult, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(stdout, maxOracleOutput+1))
		// A child that keeps writing past the bound would block forever on a
		// full pipe, so stop it here rather than in Wait.
		if len(data) > maxOracleOutput {
			kill()
		}
		outCh <- readResult{data, err}
	}()

	out := <-outCh
	waitErr := cmd.Wait()

	if len(out.data) > maxOracleOutput {
		return Resolved{}, errOutputTooLarge
	}
	if out.err != nil {
		return Resolved{}, fmt.Errorf("reading configuration from %s: %w", bin, out.err)
	}
	if waitErr != nil {
		// A declared failure is an envelope on this same stdout, so a non-zero
		// exit is read rather than reported blind. decodeFailure falls back to
		// the exit status when there is no envelope to read, and the bytes
		// never reach an error either way.
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return Resolved{}, decodeFailure(out.data, exitErr.ExitCode())
		}
		return Resolved{}, fmt.Errorf("formae could not resolve the configuration: %w", waitErr)
	}

	return decodeResolved(out.data)
}

// resolvedView is the subset of the machine view this code reads. Unknown
// fields are ignored so the producer can add fields without a break — the
// hosted arm's `auth` object is one such field, reduced to a type discriminator
// this consumer has no use for.
//
// The connection is at the top level. `profile show` nests it under `cli`;
// `connection resolve` does not, and reading the wrong shape would silently
// yield no connection at all.
type resolvedView struct {
	SchemaVersion *int   `json:"schemaVersion"`
	Profile       string `json:"profile"`
	Connection    *struct {
		Mode         string `json:"mode"`
		URL          string `json:"url"`
		Port         int    `json:"port"`
		Endpoint     string `json:"endpoint"`
		Installation string `json:"installation"`
	} `json:"connection"`
	Credential string `json:"credential"`
}

func decodeResolved(data []byte) (Resolved, error) {
	var v resolvedView
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&v); err != nil {
		return Resolved{}, errors.New("formae returned output this version cannot read")
	}
	// The machine view is exactly one document. Anything after it means we are
	// not reading what we think we are reading.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Resolved{}, errors.New("formae returned more than one document")
	}
	if v.SchemaVersion == nil {
		return Resolved{}, errors.New("formae output carries no schemaVersion")
	}
	if *v.SchemaVersion != schemaVersion {
		return Resolved{}, fmt.Errorf(
			"formae output uses schema version %d; this build understands %d",
			*v.SchemaVersion, schemaVersion)
	}
	if v.Connection == nil {
		return Resolved{}, errors.New("formae output carries no connection")
	}

	switch v.Connection.Mode {
	case "classic":
		// Classic gets a minimum check too. An empty URL would produce a
		// valid-looking connection and defer the failure to request
		// construction, which reports it far from its cause.
		if v.Connection.URL == "" {
			return Resolved{}, errors.New("formae reported a classic connection with no url")
		}
		if v.Connection.Port <= 0 {
			return Resolved{}, fmt.Errorf("formae reported an unusable port %d", v.Connection.Port)
		}
		// The MCP sends a self-hosted agent no credential. Refusing one here
		// rather than dropping it keeps that non-goal out of reach of a
		// producer bug, instead of leaving a token in a value that might later
		// grow a path to a header.
		if v.Credential != "" {
			return Resolved{}, errors.New("formae reported a credential for a classic connection")
		}
		return Resolved{
			Profile: v.Profile,
			Conn:    Classic{URL: v.Connection.URL, Port: v.Connection.Port},
		}, nil
	case "hosted":
		h := Hosted{
			Endpoint:     v.Connection.Endpoint,
			Installation: v.Connection.Installation,
		}
		if err := ValidateHosted(h); err != nil {
			return Resolved{}, fmt.Errorf("hosted connection is not usable: %w", err)
		}
		// A hosted connection that cannot be authenticated is not a usable
		// connection. The producer refuses to emit one; this refuses to invent
		// one rather than deferring the failure into a remote 401.
		if v.Credential == "" {
			return Resolved{}, errors.New("formae reported a hosted connection with no credential")
		}
		return Resolved{
			Profile:    v.Profile,
			Conn:       h,
			Credential: secret.New(v.Credential),
		}, nil
	default:
		return Resolved{}, fmt.Errorf("formae reported an unknown connection mode %q", v.Connection.Mode)
	}
}

// Resolve reads a profile's resolved connection and credential from the CLI. An
// empty profileName lets the CLI resolve the active profile and report which one
// it used, so this package never reasons about what "active" meant.
//
// forceRefresh asks the auth plugin for a fresh credential rather than the
// stored one. It is for the 401 path, which re-resolves and then checks that
// the target did not move.
func Resolve(ctx context.Context, bin, profileName string, forceRefresh bool) (Resolved, error) {
	if err := featuregate.GuardFeatureContext(ctx, featuregate.FeatureConnectionOracle, bin); err != nil {
		return Resolved{}, err
	}
	return resolveVia(ctx, bin, profileName, forceRefresh)
}
