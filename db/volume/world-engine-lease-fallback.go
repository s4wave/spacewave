//go:build wasip1

package volume

// NewFileWorldEngineLeaseProvider constructs the in-memory lease provider for
// the single-context WASI runtime.
func NewFileWorldEngineLeaseProvider(string, string) WorldEngineLeaseProvider {
	return NewInMemoryWorldEngineLeaseProvider()
}
