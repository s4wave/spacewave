//go:build js

package bldr_buildbudget

// JavaScript has no portable host-memory probe. Report the minimum virtual
// capacity that admits one GoScript compile.
func availableHostMemoryBytes() (uint64, error) {
	return uint64(GoScriptCompileWeight*defaultBudgetFractionDenominator) * (1 << 30), nil
}
