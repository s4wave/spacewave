package forge_lib_git_lazyrepo

import (
	"context"

	"github.com/s4wave/spacewave/db/unixfs"
	forge_lib_git_allocation "github.com/s4wave/spacewave/forge/lib/git/allocation"
)

// AllocationResult contains allocation provenance and a way to open writable
// cursors.
type AllocationResult struct {
	// Allocation is the Forge/Git allocation record when one was persisted.
	Allocation *forge_lib_git_allocation.Allocation
	// AllocationObjectKey is the allocation object key.
	AllocationObjectKey string
	// EvidenceObjectKey points at raw provider Evidence when present.
	EvidenceObjectKey string
	// OpenCursor opens a fresh writable cursor at the allocated repo root.
	OpenCursor func(ctx context.Context) (unixfs.FSCursor, error)
}
