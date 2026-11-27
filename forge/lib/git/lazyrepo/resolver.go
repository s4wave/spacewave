package forge_lib_git_lazyrepo

import (
	"path"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
)

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
	// Require repository mounts for path provenance resolution.
	if len(mounts) == 0 {
		return nil, errors.New("no repo mounts configured")
	}

	// Normalize the configured repository roots and guest mount paths.
	nmounts := make([]normalizedRepoMount, 0, len(mounts))
	for _, mount := range mounts {
		// Require the repository identity and pinned base commit.
		if mount.RepoObjectKey == "" {
			return nil, errors.New("repo object key cannot be empty")
		}
		if mount.BaseCommitHash == "" {
			return nil, errors.New("base commit hash cannot be empty")
		}

		// Validate the repository path within the mounted tree.
		repoRootParts, err := unixfs.CleanSplitValidateRelativePath(mount.RepoRootPath)
		if err != nil {
			return nil, errors.Wrapf(err, "repo root path %q", mount.RepoRootPath)
		}

		// Normalize the guest mount path and default the allocation path family.
		mountPath := mount.MountPath
		if mountPath == "" {
			mountPath = "/"
		}
		mountPathParts, _ := unixfs.SplitPath(path.Clean(mountPath))
		if mount.PathFamily == "" {
			mount.PathFamily = unixfs.JoinPath(repoRootParts, false)
		}

		// Retain the normalized mount and repository paths.
		nmounts = append(nmounts, normalizedRepoMount{
			input:          mount,
			mountPathParts: mountPathParts,
			repoRootParts:  repoRootParts,
		})
	}

	// Prefer the most specific mount and repository root during resolution.
	slices.SortFunc(nmounts, func(a, b normalizedRepoMount) int {
		alen := len(a.mountPathParts) + len(a.repoRootParts)
		blen := len(b.mountPathParts) + len(b.repoRootParts)
		return blen - alen
	})

	return &MountedRepoResolver{mounts: nmounts}, nil
}

// ResolveCursorPath resolves a mounted UnixFS path for the attempted operation.
// An empty operation resolves a path without mutation context.
func (r *MountedRepoResolver) ResolveCursorPath(pathParts []string, operation string) (ResolvedPath, error) {
	// Normalize the requested path before matching repository roots.
	cleanPath, err := cleanPathParts(pathParts)
	if err != nil {
		return ResolvedPath{}, err
	}

	// Resolve the most specific configured repository root.
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

	// Report the attempted mutation when no repository owns this path.
	return ResolvedPath{}, &ProvenanceError{
		Operation:    operation,
		MutationPath: unixfs.JoinPath(cleanPath, false),
		Reason:       "path is outside configured repo roots",
	}
}

// ResolveGuestPath resolves a guest filesystem path through mount provenance.
func (r *MountedRepoResolver) ResolveGuestPath(rawPath string) (ResolvedPath, error) {
	// Find the most specific mount containing the guest path.
	guestParts, _ := unixfs.SplitPath(path.Clean(rawPath))
	for _, mount := range r.mounts {
		if !hasPathPrefix(guestParts, mount.mountPathParts) {
			continue
		}
		cursorPath := slices.Clone(guestParts[len(mount.mountPathParts):])
		if !hasPathPrefix(cursorPath, mount.repoRootParts) {
			continue
		}
		resolved, err := r.ResolveCursorPath(cursorPath, "")
		if err != nil {
			return ResolvedPath{}, err
		}
		resolved.GuestPath = joinAbsPath(guestParts)
		return resolved, nil
	}

	// Report guest paths outside every configured mount.
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
