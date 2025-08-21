//go:build tools
// +build tools

// Package tools manages build-time dependencies and development tools.
// This package imports tools that are needed for code generation and testing
// but are not part of the runtime dependencies.
package tools

import (
	_ "github.com/vektra/mockery/v2"
)
