package unixfs_block

import (
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// NewFSPath builds a new filesystem path.
func NewFSPath(path []string) *FSPath {
	return &FSPath{
		Nodes: path,
	}
}

// SplitFSPath splits a path string and returns a FSPath.
func SplitFSPath(tpath string) *FSPath {
	nodes := unixfs.SplitPath(tpath)
	return NewFSPath(nodes)
}

// Validate validates the path.
func (p *FSPath) Validate() error {
	if len(p.GetNodes()) == 0 {
		return unixfs_errors.ErrEmptyPath
	}
	for _, dir := range p.GetNodes() {
		if err := ValidateDirectoryName(dir); err != nil {
			return err
		}
	}
	return nil
}

// Clone copies the path in memory.
func (p *FSPath) Clone() *FSPath {
	if p == nil {
		return nil
	}
	nodes := p.GetNodes()
	pnodes := make([]string, len(nodes))
	for i := range pnodes {
		pnodes[i] = nodes[i]
	}
	return &FSPath{Nodes: pnodes}
}

// PathsToStringSlices converts a set of paths to a list of string slices.
func PathsToStringSlices(paths ...*FSPath) [][]string {
	out := make([][]string, len(paths))
	for i, x := range paths {
		out[i] = x.GetNodes()
	}
	return out
}

// StringSlicesToPaths converts the string slices to paths.
func StringSlicesToPaths(paths [][]string) []*FSPath {
	out := make([]*FSPath, len(paths))
	for i, p := range paths {
		out[i] = NewFSPath(p)
	}
	return out
}

// PathContains checks if parentPath contains targetPath.
// Returns true if the paths are equal.
func PathContains(parentPath, targetPath []string) bool {
	if len(parentPath) > len(targetPath) {
		return false
	}

	for i, pathPt := range parentPath {
		if targetPath[i] != pathPt {
			return false
		}
	}

	return true
}
