package unixfs_block

import (
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// NewFSPath builds a new filesystem path.
func NewFSPath(path []string, isAbsolute bool) *FSPath {
	return &FSPath{
		Nodes:    path,
		Absolute: isAbsolute,
	}
}

// SplitFSPath splits a path string and returns a FSPath.
func SplitFSPath(tpath string) *FSPath {
	return NewFSPath(unixfs.SplitPath(tpath))
}

// IsNil returns if the object is nil.
func (p *FSPath) IsNil() bool {
	return p == nil
}

// Validate validates the path.
func (p *FSPath) Validate(allowEmpty bool, allowAbsolute bool) error {
	if len(p.GetNodes()) == 0 {
		if !allowEmpty {
			return unixfs_errors.ErrEmptyPath
		}
		return nil
	}
	if !allowAbsolute && p.GetAbsolute() {
		return unixfs_errors.ErrAbsolutePath
	}
	for _, dir := range p.GetNodes() {
		if err := ValidateDirentName(dir); err != nil {
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
// Assumes that all of the paths are relative.
func PathsToStringSlices(paths ...*FSPath) [][]string {
	out := make([][]string, len(paths))
	for i, x := range paths {
		out[i] = x.GetNodes()
	}
	return out
}

// StringSlicesToPaths converts the string slices to paths.
// Assumes that all the paths are relative.
func StringSlicesToPaths(paths [][]string) []*FSPath {
	out := make([]*FSPath, len(paths))
	for i, p := range paths {
		out[i] = NewFSPath(p, false)
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
