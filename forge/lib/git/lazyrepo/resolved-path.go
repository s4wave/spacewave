package forge_lib_git_lazyrepo

// ResolvedPath is the provenance for a mutation path under a mounted repo root.
type ResolvedPath struct {
	// CursorPath is the mutation path relative to the mounted UnixFS tree.
	CursorPath []string
	// GuestPath is the mutation path in the guest filesystem.
	GuestPath string
	// RepoRootPath is the repo/submodule root relative to the mounted tree.
	RepoRootPath string
	// RepoRelativePath is the path below RepoRootPath.
	RepoRelativePath []string
	// RepoObjectKey is the Spacewave Git Repo object key.
	RepoObjectKey string
	// BaseCommitHash is the pinned base commit for allocation.
	BaseCommitHash string
	// PathFamily is the allocator path family.
	PathFamily string
	// Mount is the mount provenance for this path.
	Mount MountProvenance
}
