package quadscan

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestQuadScan checks direct and wrapped predicate-only quads are flagged, in
// the wrapper's package and through its fact in another, and that a quad
// fixing the subject or the object is not.
func TestQuadScan(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "example", "example2")
}
