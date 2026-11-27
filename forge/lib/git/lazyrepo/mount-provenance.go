package forge_lib_git_lazyrepo

// MountProvenance identifies the v86fs/UnixFS mount a repo path came from.
type MountProvenance struct {
	// MountName is the v86fs mount name.
	MountName string
	// MountPath is the guest path where the UnixFS tree is mounted.
	MountPath string
}
