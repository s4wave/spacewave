// Package cdn_mirror copies a block tree from a local store into a CDN Space,
// uploading only the blocks the Space lacks, then points the Space's checkpoint
// at the tree.
//
// The copy writes children before parents, packs blocks in that order, and
// commits the packs in that order while several upload at once. Every block
// the Space lists therefore has its whole subtree listed, so a copy that is
// cut resumes by skipping each subtree whose root the Space already holds,
// and the checkpoint, posted last, names a tree that is complete.
package cdn_mirror

import (
	"context"

	"github.com/pkg/errors"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	spacewave_provider "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/hash"
	"golang.org/x/sync/errgroup"
)

// DefaultUploads is the number of packs a mirror uploads at once when the
// caller sets none, as the release publisher does.
const DefaultUploads = 4

// Options configures a mirror.
type Options struct {
	// Publish names the destination Space, its pack base URL and owner key.
	// Its Client must be a cdn_publish.StagingClient.
	Publish cdn_publish.Options
	// Uploads is the number of packs uploaded at once. Zero selects
	// DefaultUploads.
	Uploads int
	// MaxPackBytes bounds each pack. Zero selects delta.DefaultMaxChunkBytes.
	MaxPackBytes int64
	// Grants publishes to a private destination: the mirror holds a grant
	// that may write from before its first request, reads the Space's pack
	// indexes with it, and renews it before each upload and the checkpoint, so
	// the writer admission it carries never lapses. Nil reads the indexes
	// anonymously from Publish.CdnBaseURL.
	Grants cdn_bstore.GrantSource
}

// Result describes a mirror.
type Result struct {
	// Blocks and Bytes count the blocks uploaded and their bytes.
	Blocks, Bytes int64
	// Packs counts the packs committed.
	Packs int
	// Checkpoint is the checkpoint posted, or nil when the Space's checkpoint
	// already named the tree.
	Checkpoint *sobject.SOCheckpointInner
}

// MirrorWorld mirrors the committed state of eng: the World root and every
// object under it.
func MirrorWorld(ctx context.Context, eng world.Engine, opts Options) (*Result, error) {
	var res *Result
	err := eng.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		var err error
		res, err = Mirror(ctx, cursor.GetBlockStore(), cursor.GetRefWithOpArgs(), opts)
		return err
	})
	return res, err
}

// Mirror copies the tree under head from src into the destination Space, then
// posts a checkpoint naming head. src must report each block's refs, as a
// Volume does. The caller must be the Space's only writer while it runs.
func Mirror(ctx context.Context, src block.StoreOps, head *bucket.ObjectRef, opts Options) (*Result, error) {
	// Require a destination and a tree.
	pub := opts.Publish
	if pub.Client == nil || pub.DstSpaceID == "" || pub.OwnerKeyPem == "" || pub.CdnBaseURL == "" {
		return nil, errors.New("session, destination Space, owner key, and CDN URL are required")
	}
	if head.GetRootRef().GetEmpty() {
		return nil, errors.New("the mirrored tree has no root")
	}
	if _, ok := pub.Client.(cdn_publish.StagingClient); !ok {
		return nil, errors.New("the session client cannot stage a pack")
	}

	// Hold a writing grant before the first request, since it admits us.
	var grants *cdn_bstore.GrantKeeper
	if opts.Grants != nil {
		grants = cdn_bstore.NewGrantKeeper(opts.Grants, pub.DstSpaceID)
		if err := requireWrite(ctx, grants); err != nil {
			return nil, err
		}
	}

	// Skip the copy when the Space's checkpoint already names the tree. A
	// published head carries no bucket binding.
	current, err := destinationHead(ctx, pub)
	if err != nil {
		return nil, err
	}
	res := &Result{}
	unbound := head.CloneVT()
	unbound.BucketId = ""
	if current.EqualVT(unbound) {
		return res, nil
	}

	// Ask the Space which blocks it holds through its listed packs' indexes.
	entries, err := cdn_publish.FetchPackEntries(ctx, pub.Client, pub.DstSpaceID)
	if err != nil {
		return nil, err
	}
	opener := cdn_bstore.NewAnonymousOpener(nil, pub.CdnBaseURL, pub.DstSpaceID)
	if grants != nil {
		opener = grants.Opener(nil)
	}
	remote := packfile_store.NewPackfileStore(opener, nil)
	defer remote.Close()
	remote.UpdateManifest(entries)

	// Copy, pack and commit the missing blocks, then post the checkpoint.
	if err := copyTree(ctx, src, remote, head.GetRootRef(), opts, grants, res); err != nil {
		return nil, err
	}
	if err := requireWrite(ctx, grants); err != nil {
		return nil, err
	}
	res.Checkpoint, err = cdn_publish.PostCheckpoint(ctx, pub, head)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// requireWrite renews the grant when it is due and requires that it may
// write. A nil keeper requires nothing.
func requireWrite(ctx context.Context, grants *cdn_bstore.GrantKeeper) error {
	// Renew the grant if due, and refuse one that may not write.
	if grants == nil {
		return nil
	}
	g, err := grants.Current(ctx)
	if err != nil {
		return err
	}
	if !g.GetMayWrite() {
		return errors.New("the grant does not allow publishing to the Space")
	}
	return nil
}

// destinationHead returns the head the Space's checkpoint names, or nil.
func destinationHead(ctx context.Context, pub cdn_publish.Options) (*bucket.ObjectRef, error) {
	// Read the authoritative state, since the served pointer can lag.
	data, err := pub.Client.GetSOState(ctx, pub.DstSpaceID, 0, spacewave_provider.SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "fetch destination state")
	}
	msg := &api.SOStateMessage{}
	if err := msg.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode destination state")
	}

	// Decode the head its checkpoint names.
	if msg.GetSnapshot().GetCheckpoint() == nil {
		return nil, nil
	}
	return cdn_publish.DecodeHeadRef(msg.GetSnapshot())
}

// copyTree runs the copy as three stages joined by channels: the graph copy
// writes the blocks the Space lacks in post-order, the packer cuts them into
// packs and starts each upload, and the committer commits the packs in the
// order they were cut.
func copyTree(ctx context.Context, src block.StoreOps, remote *packfile_store.PackfileStore, root *block.BlockRef, opts Options, grants *cdn_bstore.GrantKeeper, res *Result) error {
	// Bound the uploads in flight and the packs awaiting their commit.
	uploads := opts.Uploads
	if uploads <= 0 {
		uploads = DefaultUploads
	}
	g, gctx := errgroup.WithContext(ctx)
	blocks := make(chan copiedBlock)
	packs := make(chan *pendingPack, uploads)
	slots := make(chan struct{}, uploads)

	// Copy the blocks the Space lacks, skipping each subtree it holds.
	g.Go(func() error {
		defer close(blocks)
		return block.CopyGraph(gctx, src, &blockSink{out: blocks}, root, &block.GraphCopyOptions{
			Known: remote.GetBlockExistsBatch,
		})
	})

	// Cut the blocks into packs and start each pack's upload.
	g.Go(func() error {
		defer close(packs)
		_, err := delta.EmitDeltaChunks(gctx, opts.Publish.DstSpaceID, func() (*hash.Hash, *block.StoredBlock, error) {
			select {
			case b, ok := <-blocks:
				if !ok {
					return nil, nil, nil
				}
				res.Blocks++
				res.Bytes += int64(len(b.stored.Data))
				return b.hash, b.stored, nil
			case <-gctx.Done():
				return nil, nil, context.Cause(gctx)
			}
		}, opts.MaxPackBytes, func(ctx context.Context, _ int, entry *packfile.PackfileEntry, data []byte) error {
			return startUpload(ctx, g, slots, packs, opts.Publish, grants, entry, data)
		})
		return err
	})

	// Commit the packs in the order they were cut.
	g.Go(func() error {
		for p := range packs {
			select {
			case <-p.done:
			case <-gctx.Done():
				return context.Cause(gctx)
			}
			if p.staged == nil {
				return errors.New("a pack upload failed")
			}
			if err := p.staged.Commit(gctx); err != nil {
				return errors.Wrapf(err, "commit pack %s", p.staged.ID())
			}
			res.Packs++
		}
		return nil
	})
	return g.Wait()
}

// startUpload waits for an upload slot, queues the pack for its commit, and
// uploads it in the background.
func startUpload(ctx context.Context, g *errgroup.Group, slots chan struct{}, packs chan<- *pendingPack, pub cdn_publish.Options, grants *cdn_bstore.GrantKeeper, entry *packfile.PackfileEntry, data []byte) error {
	// Take an upload slot and a place in the commit order.
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	p := &pendingPack{done: make(chan struct{})}
	select {
	case packs <- p:
	case <-ctx.Done():
		<-slots
		return context.Cause(ctx)
	}

	// Upload the pack, then release its slot and its bytes.
	g.Go(func() error {
		// Free the slot and wake the committer when the upload returns.
		defer func() { <-slots }()
		defer close(p.done)

		// Upload the pack under a current admission and hand it to the
		// committer.
		if err := requireWrite(ctx, grants); err != nil {
			return err
		}
		staged, err := cdn_publish.StagePackData(ctx, pub, data, entry.GetBloomFilter())
		if err != nil {
			return errors.Wrap(err, "upload pack")
		}
		p.staged = staged
		return nil
	})
	return nil
}

// pendingPack is a pack queued for its commit.
type pendingPack struct {
	// done closes when the upload returns.
	done chan struct{}
	// staged is the uploaded pack, or nil when the upload failed.
	staged *cdn_publish.StagedPack
}

// copiedBlock is a block the graph copy wrote.
type copiedBlock struct {
	hash   *hash.Hash
	stored *block.StoredBlock
}

// blockSink is the graph copy's destination: it hands each written block to
// the packer. The copy calls no other method.
type blockSink struct {
	block.StoreOps
	// out receives each block in write order.
	out chan<- copiedBlock
}

// PutBlockBatch hands each block to the packer, in order.
func (s *blockSink) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	for _, e := range entries {
		b := copiedBlock{
			hash:   e.Ref.GetHash(),
			stored: &block.StoredBlock{Data: e.Data, Refs: e.Refs, RefsKnown: e.RefsKnown},
		}
		select {
		case s.out <- b:
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		}
	}
	return make([]bool, len(entries)), nil
}
