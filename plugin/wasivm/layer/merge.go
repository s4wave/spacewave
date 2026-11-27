package layer

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
)

// MergeLayers lays each layer onto base in order through bls and returns the
// merged root. base may be nil for an empty root. Directories present in both
// trees merge entry by entry. Any other node in a later layer replaces the
// earlier one. Every node the merge does not change keeps its reference, so
// the merge writes only the directories along the paths where layers meet.
// collide is called with the path of each node a layer replaces with
// different content.
func MergeLayers(
	ctx context.Context,
	bls *bucket_lookup.Cursor,
	base *block.BlockRef,
	layers []*block.BlockRef,
	collide func(path string),
) (*block.BlockRef, error) {
	// Open the base root, or start an empty one.
	btx, bcs := bls.BuildTransactionAtRef(nil, base)
	if base == nil {
		bcs.SetBlock(unixfs_block.NewFSNode(dirType, 0o755, epoch), true)
	}
	root, err := unixfs_block.NewFSTree(ctx, bcs, dirType)
	if err != nil {
		return nil, err
	}

	// Lay each layer onto the root and write the result.
	for _, layer := range layers {
		_, srcBcs := bls.BuildTransactionAtRef(nil, layer)
		src, err := unixfs_block.NewFSTree(ctx, srcBcs, dirType)
		if err != nil {
			return nil, err
		}
		if err := mergeDir(root, src, "", collide); err != nil {
			return nil, err
		}
	}
	ref, _, err := btx.Write(ctx, true)
	return ref, err
}

// mergeDir lays the entries of directory src onto directory dst, the
// directory at path dir.
func mergeDir(dst, src *unixfs_block.FSTree, dir string, collide func(path string)) error {
	for _, ent := range src.GetFSNode().GetDirectoryEntry() {
		name := ent.GetName()
		path := dir + "/" + name

		// Merge two directories entry by entry.
		dstChild, dstEnt, err := dst.LookupFollowDirent(name)
		if err != nil {
			return err
		}
		if dstEnt != nil && dstEnt.GetIsDirectory() && ent.GetIsDirectory() {
			srcChild, _, err := src.LookupFollowDirent(name)
			if err != nil {
				return err
			}
			if err := mergeDir(dstChild, srcChild, path, collide); err != nil {
				return errors.Wrap(err, name)
			}
			continue
		}

		// Replace any other existing node with a reference to the layer's node.
		if dstEnt != nil {
			if !dstEnt.GetNodeRef().EqualsRef(ent.GetNodeRef()) {
				collide(path)
			}
			if _, err := dst.Remove([]string{name}, nil); err != nil {
				return err
			}
		}
		if _, err := dst.Mknod(name, ent.GetNodeType(), ent.GetNodeRef(), 0, nil); err != nil {
			return errors.Wrap(err, name)
		}
	}
	return nil
}
