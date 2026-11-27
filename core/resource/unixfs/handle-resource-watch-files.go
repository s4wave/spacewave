package resource_unixfs

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
)

// defaultWatchFilesLimit bounds the paths WatchFiles matches when the request
// sets no limit.
const defaultWatchFilesLimit = 10000

// WatchFiles streams the files below the directory whose names end in one of
// the requested extensions. The first response holds every match, and each
// later response holds the paths added and removed by a change.
//
// The watch keeps the scan of each directory with its block reference. A
// change rescans only the directories whose reference changed, so an edit
// reads the directories on its path and reuses every other subtree.
func (r *FSHandleResource) WatchFiles(req *s4wave_unixfs.HandleWatchFilesRequest, strm s4wave_unixfs.SRPCFSHandleResourceService_WatchFilesStream) error {
	// Normalize the match rules.
	ctx := strm.Context()
	extensions := make([]string, 0, len(req.GetExtensions()))
	for _, ext := range req.GetExtensions() {
		if ext != "" {
			extensions = append(extensions, strings.ToLower(ext))
		}
	}
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = defaultWatchFilesLimit
	}

	// Scan after each change, and send the difference from the last result.
	var root *watchFilesDir
	var sent, next *watchFilesResult
	return r.watchChanges(
		ctx,
		func(handle *unixfs.FSHandle) error {
			// Rescan the changed directories and collect the matches.
			var err error
			root, err = scanWatchFilesDir(ctx, handle, root, extensions)
			if err != nil {
				return err
			}
			next = root.collect(limit)
			return nil
		},
		func() error {
			resp, changed := next.diff(sent)
			if !changed {
				return nil
			}
			sent = next
			return strm.Send(resp)
		},
	)
}

// watchFilesDir is the scan of one directory below a WatchFiles root.
type watchFilesDir struct {
	// ref is the block reference of the scanned directory node, or empty when
	// the handle does not expose one.
	ref *block.BlockRef
	// entries are the matched files and the subdirectories, in path order.
	entries []watchFilesEntry
	// err is the error that stopped reading the directory.
	err error
}

// watchFilesEntry is one matched file or subdirectory of a directory.
type watchFilesEntry struct {
	// name is the entry name.
	name string
	// dir is the scan of a subdirectory, or nil for a matched file.
	dir *watchFilesDir
}

// scanWatchFilesDir scans the directory at handle for files that match
// extensions. It returns prev when the directory's block reference equals the
// one prev was scanned at, and reuses the unchanged subtrees of prev otherwise.
//
// A directory that cannot be read is recorded in the scan. Only the release
// of the handle or the end of ctx stop the scan with an error.
func scanWatchFilesDir(ctx context.Context, handle *unixfs.FSHandle, prev *watchFilesDir, extensions []string) (*watchFilesDir, error) {
	// Reuse the previous scan while the subtree is unchanged.
	var prevRef *block.BlockRef
	if prev != nil {
		prevRef = prev.ref
	}
	entries, ref, changed, err := readWatchEntries(ctx, handle, prevRef)
	if err != nil {
		return watchFilesDirError(ctx, err)
	}
	if !changed {
		return prev, nil
	}

	// Keep the matched files, and scan each subdirectory against its previous
	// scan.
	dir := &watchFilesDir{ref: ref}
	for _, ent := range entries {
		name := ent.GetName()
		switch {
		case ent.GetIsDir():
			child, err := scanWatchFilesChild(ctx, handle, name, prev.child(name), extensions)
			if err != nil {
				return nil, err
			}
			dir.entries = append(dir.entries, watchFilesEntry{name: name, dir: child})
		case !ent.GetIsSymlink() && matchesExtension(name, extensions):
			dir.entries = append(dir.entries, watchFilesEntry{name: name})
		}
	}

	// Order the entries by the paths they lead to.
	slices.SortFunc(dir.entries, func(a, b watchFilesEntry) int {
		return cmp.Compare(a.pathKey(), b.pathKey())
	})
	return dir, nil
}

// scanWatchFilesChild scans the subdirectory name of the directory at handle.
func scanWatchFilesChild(ctx context.Context, handle *unixfs.FSHandle, name string, prev *watchFilesDir, extensions []string) (*watchFilesDir, error) {
	child, err := handle.Lookup(ctx, name)
	if err != nil {
		return watchFilesDirError(ctx, err)
	}
	defer child.Release()
	return scanWatchFilesDir(ctx, child, prev, extensions)
}

// watchFilesDirError returns err when it stops the scan, and otherwise a scan
// that records the directory as unreadable.
func watchFilesDirError(ctx context.Context, err error) (*watchFilesDir, error) {
	if errors.Is(err, unixfs_errors.ErrReleased) || ctx.Err() != nil {
		return nil, err
	}
	return &watchFilesDir{err: err}, nil
}

// child returns the scan of the subdirectory name, or nil when d has none.
func (d *watchFilesDir) child(name string) *watchFilesDir {
	if d == nil {
		return nil
	}
	for _, ent := range d.entries {
		if ent.name == name {
			return ent.dir
		}
	}
	return nil
}

// pathKey returns the key that orders the entry among the paths below its
// directory: the name of a file, and the name and separator of a directory.
func (e watchFilesEntry) pathKey() string {
	if e.dir != nil {
		return e.name + "/"
	}
	return e.name
}

// matchesExtension reports whether name ends in one of extensions, ignoring
// case.
func matchesExtension(name string, extensions []string) bool {
	lower := strings.ToLower(name)
	return slices.ContainsFunc(extensions, func(ext string) bool {
		return strings.HasSuffix(lower, ext)
	})
}

// watchFilesResult is the matched paths and read errors of one scan.
type watchFilesResult struct {
	// paths are the first matched paths in path order, up to the limit.
	paths []string
	// truncated is set when the limit hid further matches.
	truncated bool
	// errors describe the directories that could not be read.
	errors []string
}

// collect returns the first limit matched paths below d and the read errors.
func (d *watchFilesDir) collect(limit int) *watchFilesResult {
	res := &watchFilesResult{}
	d.collectInto(res, "", limit)
	return res
}

// collectInto appends the matches and errors below d, whose path is prefix, to
// res until it holds limit paths.
func (d *watchFilesDir) collectInto(res *watchFilesResult, prefix string, limit int) {
	// Report a directory that could not be read, naming the root ".".
	if d.err != nil {
		dirPath := strings.TrimSuffix(prefix, "/")
		if dirPath == "" {
			dirPath = "."
		}
		res.errors = append(res.errors, dirPath+": "+d.err.Error())
		return
	}

	// Walk the entries in path order until the path limit.
	for _, ent := range d.entries {
		if res.truncated {
			return
		}
		if ent.dir != nil {
			ent.dir.collectInto(res, prefix+ent.name+"/", limit)
			continue
		}
		if len(res.paths) == limit {
			res.truncated = true
			return
		}
		res.paths = append(res.paths, prefix+ent.name)
	}
}

// diff returns the response that changes prev into res, and whether it changes
// anything. A nil prev is the state before the first response, and the watch
// always sends that first response.
func (res *watchFilesResult) diff(prev *watchFilesResult) (*s4wave_unixfs.HandleWatchFilesResponse, bool) {
	// Merge the two sorted path lists.
	resp := &s4wave_unixfs.HandleWatchFilesResponse{Truncated: res.truncated, Errors: res.errors}
	var prevPaths []string
	if prev != nil {
		prevPaths = prev.paths
	}
	i, j := 0, 0
	for i < len(res.paths) || j < len(prevPaths) {
		switch {
		case j == len(prevPaths) || (i < len(res.paths) && res.paths[i] < prevPaths[j]):
			resp.Added = append(resp.Added, res.paths[i])
			i++
		case i == len(res.paths) || prevPaths[j] < res.paths[i]:
			resp.Removed = append(resp.Removed, prevPaths[j])
			j++
		default:
			i++
			j++
		}
	}

	// Report a change of the paths, the truncation, or the errors.
	changed := prev == nil ||
		len(resp.Added) != 0 ||
		len(resp.Removed) != 0 ||
		res.truncated != prev.truncated ||
		!slices.Equal(res.errors, prev.errors)
	return resp, changed
}
