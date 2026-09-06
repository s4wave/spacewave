//go:build js

package blockshard

// manifestSnapshot returns the current immutable generation for package-local
// reads. Callers must neither mutate it (including nested key slices) nor expose
// it through a public API. Writers clone before changing a generation and replace
// the published pointer under mu. Holding this pointer preserves metadata only;
// segment access still requires a lease and the normal missing-file refresh.
func (s *Shard) manifestSnapshot() *Manifest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.manifest
}
