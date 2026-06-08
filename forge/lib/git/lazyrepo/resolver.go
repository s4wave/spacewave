package forge_lib_git_lazyrepo

import (
	"path"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
)

// MountProvenance identifies the v86fs/UnixFS mount a repo path came from.
type MountProvenance struct {
	// MountName is the v86fs mount name.
	MountName string
	// MountPath is the guest path where the UnixFS tree is mounted.
	MountPath string
}

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

type normalizedRepoMount struct {
	input          RepoMount
	mountPathParts []string
	repoRootParts  []string
}

// MountedRepoResolver resolves mutation paths to repo/submodule mount provenance.
type MountedRepoResolver struct {
	mounts []normalizedRepoMount
}

// NewMountedRepoResolver constructs a resolver from mounted repo roots.
func NewMountedRepoResolver(mounts []RepoMount) (*MountedRepoResolver, error) {
	if len(mounts) == 0 {
		return nil, errors.New("no repo mounts configured")
	}
	nmounts := make([]normalizedRepoMount, 0, len(mounts))
	for _, mount := range mounts {
		if mount.RepoObjectKey == "" {
			return nil, errors.New("repo object key cannot be empty")
		}
		if mount.BaseCommitHash == "" {
			return nil, errors.New("base commit hash cannot be empty")
		}
		repoRootParts, err := unixfs.CleanSplitValidateRelativePath(mount.RepoRootPath)
		if err != nil {
			return nil, errors.Wrapf(err, "repo root path %q", mount.RepoRootPath)
		}
		mountPath := mount.MountPath
		if mountPath == "" {
			mountPath = "/"
		}
		mountPathParts, _ := unixfs.SplitPath(path.Clean(mountPath))
		if mount.PathFamily == "" {
			mount.PathFamily = unixfs.JoinPath(repoRootParts, false)
		}
		nmounts = append(nmounts, normalizedRepoMount{
			input:          mount,
			mountPathParts: mountPathParts,
			repoRootParts:  repoRootParts,
		})
	}
	slices.SortFunc(nmounts, func(a, b normalizedRepoMount) int {
		alen := len(a.mountPathParts) + len(a.repoRootParts)
		blen := len(b.mountPathParts) + len(b.repoRootParts)
		return blen - alen
	})
	return &MountedRepoResolver{mounts: nmounts}, nil
}

// ResolveCursorPath resolves a path relative to the mounted UnixFS tree.
func (r *MountedRepoResolver) ResolveCursorPath(pathParts []string) (ResolvedPath, error) {
	cleanPath, err := cleanPathParts(pathParts)
	if err != nil {
		return ResolvedPath{}, err
	}
	for _, mount := range r.mounts {
		if !hasPathPrefix(cleanPath, mount.repoRootParts) {
			continue
		}
		rel := slices.Clone(cleanPath[len(mount.repoRootParts):])
		return ResolvedPath{
			CursorPath:       cleanPath,
			GuestPath:        joinGuestPath(mount.mountPathParts, cleanPath),
			RepoRootPath:     unixfs.JoinPath(mount.repoRootParts, false),
			RepoRelativePath: rel,
			RepoObjectKey:    mount.input.RepoObjectKey,
			BaseCommitHash:   mount.input.BaseCommitHash,
			PathFamily:       mount.input.PathFamily,
			Mount: MountProvenance{
				MountName: mount.input.MountName,
				MountPath: joinAbsPath(mount.mountPathParts),
			},
		}, nil
	}
	return ResolvedPath{}, &ProvenanceError{
		MutationPath: unixfs.JoinPath(cleanPath, false),
		Reason:       "path is outside configured repo roots",
	}
}

// ResolveGuestPath resolves a guest filesystem path through mount provenance.
func (r *MountedRepoResolver) ResolveGuestPath(rawPath string) (ResolvedPath, error) {
	guestParts, _ := unixfs.SplitPath(path.Clean(rawPath))
	for _, mount := range r.mounts {
		if !hasPathPrefix(guestParts, mount.mountPathParts) {
			continue
		}
		cursorPath := slices.Clone(guestParts[len(mount.mountPathParts):])
		if !hasPathPrefix(cursorPath, mount.repoRootParts) {
			continue
		}
		resolved, err := r.ResolveCursorPath(cursorPath)
		if err != nil {
			return ResolvedPath{}, err
		}
		resolved.GuestPath = joinAbsPath(guestParts)
		return resolved, nil
	}
	return ResolvedPath{}, &ProvenanceError{
		MutationPath: joinAbsPath(guestParts),
		Reason:       "path is outside configured mounts",
	}
}

func cleanPathParts(pathParts []string) ([]string, error) {
	clean, err := unixfs.CleanSplitValidateRelativePath(unixfs.JoinPath(pathParts, false))
	if err != nil {
		return nil, err
	}
	return clean, nil
}

func hasPathPrefix(pathParts, prefix []string) bool {
	if len(pathParts) < len(prefix) {
		return false
	}
	for idx, part := range prefix {
		if pathParts[idx] != part {
			return false
		}
	}
	return true
}

func joinGuestPath(mountPathParts, cursorPath []string) string {
	parts := make([]string, 0, len(mountPathParts)+len(cursorPath))
	parts = append(parts, mountPathParts...)
	parts = append(parts, cursorPath...)
	return joinAbsPath(parts)
}

func joinAbsPath(parts []string) string {
	if len(parts) == 0 {
		return "/"
	}
	clean := path.Clean("/" + strings.Join(parts, "/"))
	if clean == "." {
		return "/"
	}
	return clean
}

func allocationKey(resolved ResolvedPath) string {
	return strings.Join([]string{
		resolved.RepoObjectKey,
		resolved.BaseCommitHash,
		resolved.PathFamily,
	}, "\x00")
}
