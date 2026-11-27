package space_unixfs

import (
	"context"
	"slices"
	"strings"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// projectedMountName is the directory entry that opens the object whose key is
// the directory's own path.
const projectedMountName = "-"

// isProjectedObject reports whether objectKey identifies an object of a
// projected type.
func isProjectedObject(ctx context.Context, ws world.WorldState, objectKey string) (bool, error) {
	typeID, err := world_types.GetObjectType(ctx, ws, objectKey)
	if err != nil {
		return false, err
	}
	return supportsProjectedType(typeID), nil
}

// hasProjectedObject reports whether objectKey or an object below it is
// projected. It reads the objects below objectKey only until the first
// projected one.
func hasProjectedObject(ctx context.Context, ws world.WorldState, objectKey string) (bool, error) {
	// Check the object itself.
	found, err := isProjectedObject(ctx, ws, objectKey)
	if err != nil || found {
		return found, err
	}

	// Check the objects below it, never the World type records.
	prefix := objectKey + "/"
	if prefix == world_types.TypesPrefix {
		return false, nil
	}
	it := ws.IterateObjects(ctx, prefix, false)
	defer it.Close()
	for it.Next() {
		found, err := isProjectedObject(ctx, ws, it.Key())
		if err != nil || found {
			return found, err
		}
	}
	return false, it.Err()
}

// listProjectedChildren returns the sorted names of the projected directory at
// path: the next key segment of each projected object below it, and the mount
// entry when path itself identifies a projected object.
//
// It iterates only the keys below path, and skips the rest of a child's
// subtree once the child is known.
func listProjectedChildren(ctx context.Context, ws world.WorldState, path []string) ([]string, error) {
	// Iterate the objects below the directory.
	var prefix string
	if len(path) != 0 {
		prefix = strings.Join(path, "/") + "/"
	}
	it := ws.IterateObjects(ctx, prefix, false)
	defer it.Close()

	// Name each child that holds a projected object, skipping the subtrees
	// of known children and of the World type records.
	var names []string
	known := make(map[string]bool)
	valid := it.Next()
	for valid {
		// Skip a known child's subtree and keys with an empty segment.
		key := it.Key()
		name, _, nested := strings.Cut(key[len(prefix):], "/")
		group := prefix + name + "/"
		if name == "" {
			valid = it.Next()
			continue
		}
		if nested && (known[name] || group == world_types.TypesPrefix) {
			var err error
			if valid, err = seekPastPrefix(it, group); err != nil {
				return nil, err
			}
			continue
		}

		// Record the child when this object is projected.
		found, err := isProjectedObject(ctx, ws, key)
		if err != nil {
			return nil, err
		}
		if found && !known[name] {
			known[name] = true
			names = append(names, name)
		}
		valid = it.Next()
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	// Add the mount entry when the directory is itself a projected object.
	if len(path) != 0 {
		mounted, err := isProjectedObject(ctx, ws, strings.Join(path, "/"))
		if err != nil {
			return nil, err
		}
		if mounted {
			names = append(names, projectedMountName)
		}
	}
	slices.Sort(names)
	return names, nil
}

// seekPastPrefix moves it to the first key that does not start with prefix
// and reports whether it points at a key.
func seekPastPrefix(it world.ObjectIterator, prefix string) (bool, error) {
	end, ok := kvtx.PrefixSuccessor([]byte(prefix))
	if !ok {
		return false, nil
	}
	if err := it.Seek(string(end)); err != nil {
		return false, err
	}
	return it.Valid(), nil
}

// buildProjectedDirents returns a directory entry for each of names.
func buildProjectedDirents(names []string) []*projectedDirent {
	dirents := make([]*projectedDirent, 0, len(names))
	for _, name := range names {
		dirents = append(dirents, newProjectedDirent(name, unixfs.NewFSCursorNodeType_Dir()))
	}
	return dirents
}
