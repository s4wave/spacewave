package unixfs_sync

import (
	"context"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/s4wave/spacewave/db/unixfs"
)

// FilterCb is a callback that can be passed to Sync.
// If it returns false for cntu, skips the node & children.
// If it returns an error, Sync is canceled and returns the error.
// The path will be formatted like "directory/file.txt"
type FilterCb func(ctx context.Context, path string, nodeType unixfs.FSCursorNodeType) (cntu bool, err error)

// CombineFilterCbs combines multiple FilterCb into a single FilterCb.
func CombineFilterCbs(cbs ...FilterCb) FilterCb {
	// filter any nil values
	for i := 0; i < len(cbs); i++ {
		if cbs[i] == nil {
			cbs[i] = cbs[len(cbs)-1]
			cbs[len(cbs)-1] = nil
			cbs = cbs[:len(cbs)-1]
			i--
		}
	}
	if len(cbs) == 0 {
		return nil
	}
	if len(cbs) == 1 {
		return cbs[0]
	}
	return func(ctx context.Context, path string, nodeType unixfs.FSCursorNodeType) (bool, error) {
		for _, cb := range cbs {
			cntu, err := cb(ctx, path, nodeType)
			if err != nil {
				return false, err
			}
			if !cntu {
				return false, nil
			}
		}
		return true, nil
	}
}

// NewSkipPathPrefixes creates a FilterCb which skips given path prefixes from a []string slice.
func NewSkipPathPrefixes(skipPathPrefixes []string) FilterCb {
	return func(ctx context.Context, path string, _ unixfs.FSCursorNodeType) (cntu bool, err error) {
		for _, prefix := range skipPathPrefixes {
			if strings.HasPrefix(path, prefix) {
				return false, nil
			}
		}
		return true, nil
	}
}

// NewFilterFileList filters by checking if the path is in the list of paths.
// Performance is significantly better if pathList is sorted.
// Directories are never skipped.
//
// NOTE: the path list MUST be sorted in ascending order if isSorted=true!
// NOTE: the path list MUST be formatted like "directory/file.txt" with no leading ./
func NewFilterFileList(pathList []string, isSorted bool) FilterCb {
	return func(ctx context.Context, path string, nt unixfs.FSCursorNodeType) (cntu bool, err error) {
		// careful here to skip symlinks
		if nt.GetIsDirectory() {
			return true, nil
		}
		if !nt.GetIsFile() {
			return false, nil
		}

		if isSorted {
			_, cntu = slices.BinarySearch(pathList, path)
		} else {
			cntu = slices.Contains(pathList, path)
		}
		return cntu, nil
	}
}

// CleanPathListForFilter cleans the given list of paths for NewFilterFileList.
//
// Sorts and cleans the paths. Strips any leading '/'.
func CleanPathListForFilter(pathList []string) []string {
	out := make([]string, len(pathList))
	for i, srcPath := range pathList {
		srcPath = path.Clean(srcPath)
		if len(srcPath) != 0 && srcPath[0] == '/' {
			srcPath = srcPath[1:]
		}
		out[i] = srcPath
	}
	sort.Strings(out)
	out = slices.Compact(out)
	return out
}
