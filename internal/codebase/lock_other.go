//go:build !linux && !darwin

// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package codebase

import (
	"context"
	"errors"
)

func lockRegistry(context.Context, string) (func(), error) {
	return nil, errors.New("codebase registry locking is supported on Linux and macOS")
}
