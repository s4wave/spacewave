//go:build js

package bldr_buildbudget

// availableHostMemoryBytes reports the minimum virtual capacity that admits
// one GoScript compile because JavaScript has no portable host-memory probe.
func availableHostMemoryBytes() (uint64, error) {
	return uint64(GoScriptCompileWeight*defaultBudgetFractionDenominator) * (1 << 30), nil
}
