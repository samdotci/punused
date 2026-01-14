// This must be package main
package main

import (
	"github.com/bep/punused/internal/lib"
	"golang.org/x/tools/go/analysis"
)

// New creates a new punused analyzer for use as a golangci-lint plugin.
// The configuration parameter is currently not used but reserved for future use.
func New(conf any) ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{lib.Analyzer}, nil
}
