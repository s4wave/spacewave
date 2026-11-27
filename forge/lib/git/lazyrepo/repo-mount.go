package forge_lib_git_lazyrepo

// RepoMount binds a repo/submodule root inside a mounted UnixFS tree.
type RepoMount struct {
	// MountName is the v86fs mount name.
	MountName string
	// MountPath is the guest path where the UnixFS tree is mounted.
	MountPath string
	// RepoRootPath is the repo/submodule path relative to the mounted tree.
	RepoRootPath string
	// RepoObjectKey is the Spacewave Git Repo object key.
	RepoObjectKey string
	// BaseCommitHash is the pinned base commit for allocation.
	BaseCommitHash string
	// PathFamily is the allocator path family. Defaults to RepoRootPath.
	PathFamily string
}
