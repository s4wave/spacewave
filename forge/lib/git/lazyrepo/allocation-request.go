package forge_lib_git_lazyrepo

// AllocationRequest is passed to Allocator before a repo-root mutation.
type AllocationRequest struct {
	ResolvedPath
	// Operation is the mutation operation that triggered allocation.
	Operation string
}
