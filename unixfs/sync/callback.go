package unixfs_sync

import (
	"context"
	"strings"

	"github.com/aperturerobotics/hydra/unixfs"
	"golang.org/x/exp/slices"
)

// FilterCb is a callback that can be passed to Sync.
// If it returns false for cntu, skips the handle & children.
// If it returns an error, Sync is canceled and returns the error.
// Handle will be nil if the value does not exist in the source tree (deleting).
// The path will be formatted like "directory/file.txt"
type FilterCb func(ctx context.Context, path string, handle *unixfs.FSHandle) (cntu bool, err error)

// NewSkipPathPrefixes creates a FilterCb which skips given path prefixes from a []string slice.
func NewSkipPathPrefixes(skipPathPrefixes []string) FilterCb {
	return func(ctx context.Context, path string, _ *unixfs.FSHandle) (cntu bool, err error) {
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
	return func(ctx context.Context, path string, h *unixfs.FSHandle) (cntu bool, err error) {
		nt, err := h.GetNodeType(ctx)
		if err != nil {
			return false, err
		}
		if !nt.GetIsFile() {
			return true, nil
		}

		if isSorted {
			_, cntu = slices.BinarySearch(pathList, path)
		} else {
			cntu = slices.Contains(pathList, path)
		}
		return cntu, nil
	}
}
