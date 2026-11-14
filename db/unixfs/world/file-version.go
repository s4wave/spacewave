package unixfs_world

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
)

// FileVersion is the version a committed batch gave one file.
type FileVersion struct {
	// Path is the full path of the file.
	Path []string
	// Version is the node reference of the file in the committed root, as
	// ExpectFile takes it.
	Version *block.BlockRef
}

// readFileVersions reads the node reference of the file at each of paths in
// the tree at root, in order. A path that holds no file is an error: the
// caller asks only for files its batch wrote.
func readFileVersions(root *unixfs_block.FSTree, paths [][]string) ([]FileVersion, error) {
	out := make([]FileVersion, 0, len(paths))
	for _, path := range paths {
		// Find the entry at the path.
		var dirent *unixfs_block.Dirent
		dir, _, err := unixfs_block.LookupFSTreePath(root, path[:len(path)-1])
		if err == nil {
			dirent, err = dir.Lookup(path[len(path)-1])
		}
		if err != nil {
			return nil, errors.Wrapf(err, "version of %q", strings.Join(path, "/"))
		}

		// Take the node reference of the file it holds.
		ref := dirent.GetNodeRef()
		if dirent.GetNodeType() != unixfs_block.NodeType_NodeType_FILE || ref.GetEmpty() {
			return nil, errors.Errorf("committed root holds no file at %q", strings.Join(path, "/"))
		}
		out = append(out, FileVersion{Path: path, Version: ref.Clone()})
	}
	return out, nil
}
