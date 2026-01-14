package lib

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestAnalyzer(t *testing.T) {
	c := qt.New(t)

	// Test that the analyzer is correctly configured
	c.Assert(Analyzer.Name, qt.Equals, "punused")
	c.Assert(Analyzer.Doc, qt.Not(qt.Equals), "")
	c.Assert(Analyzer.Run, qt.Not(qt.IsNil))
}
