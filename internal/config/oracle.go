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

// Resolved is one profile evaluation: the effective profile name the CLI
// reported, and the connection it resolved.
type Resolved struct {
	Profile string
	Conn    Connection
}

// resolveVia runs the CLI as the configuration oracle and decodes its machine
// view. Stdout and stderr are captured separately and drained concurrently:
// combined capture would put subprocess output into errors, and reading two
// pipes serially can deadlock when the child fills the one not being read.
func resolveVia(ctx context.Context, bin, profileName string) (Resolved, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, oracleTimeout)
		defer cancel()
	}

	// --profile is a global flag and precedes the subcommand.
	args := []string{}
	if profileName != "" {
		args = append(args, "--profile", profileName)
	}
	args = append(args, "profile", "show",
		"--output-consumer", "machine", "--output-schema", "json")

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
		// Reports the failure and the exit status, never the bytes.
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return Resolved{}, fmt.Errorf("formae could not resolve the configuration (exit %d)", exitErr.ExitCode())
		}
		return Resolved{}, fmt.Errorf("formae could not resolve the configuration: %w", waitErr)
	}

	return decodeProfileShow(out.data)
}

// profileShowView is the subset of the machine view this code reads. Unknown
// fields are ignored so the producer can add fields without a break.
type profileShowView struct {
	SchemaVersion *int   `json:"schemaVersion"`
	Profile       string `json:"profile"`
	Cli           struct {
		Connection *struct {
			Mode         string `json:"mode"`
			URL          string `json:"url"`
			Port         int    `json:"port"`
			Endpoint     string `json:"endpoint"`
			Installation string `json:"installation"`
		} `json:"connection"`
	} `json:"cli"`
}

func decodeProfileShow(data []byte) (Resolved, error) {
	var v profileShowView
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
	if v.Cli.Connection == nil {
		return Resolved{}, errors.New("formae output carries no cli.connection")
	}

	switch v.Cli.Connection.Mode {
	case "classic":
		// Classic gets a minimum check too. An empty URL would produce a
		// valid-looking connection and defer the failure to request
		// construction, which reports it far from its cause.
		if v.Cli.Connection.URL == "" {
			return Resolved{}, errors.New("formae reported a classic connection with no url")
		}
		if v.Cli.Connection.Port <= 0 {
			return Resolved{}, fmt.Errorf("formae reported an unusable port %d", v.Cli.Connection.Port)
		}
		return Resolved{
			Profile: v.Profile,
			Conn:    Classic{URL: v.Cli.Connection.URL, Port: v.Cli.Connection.Port},
		}, nil
	case "hosted":
		h := Hosted{
			Endpoint:     v.Cli.Connection.Endpoint,
			Installation: v.Cli.Connection.Installation,
		}
		if err := ValidateHosted(h); err != nil {
			return Resolved{}, fmt.Errorf("hosted connection is not usable: %w", err)
		}
		return Resolved{Profile: v.Profile, Conn: h}, nil
	default:
		return Resolved{}, fmt.Errorf("formae reported an unknown connection mode %q", v.Cli.Connection.Mode)
	}
}
