package publisher

import (
	"context"

	"github.com/pkg/errors"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// packUploads bounds the release packs uploaded at once.
const packUploads = 4

// Publish uploads a committed release World, advances its public checkpoint,
// then rewrites the destination packs that are mostly unreachable from it.
// Every referenced block must exist before any network write begins. Failed
// uploads leave the previous checkpoint intact. Content-addressed packs are
// retryable.
// The caller must exclusively own the local World for the operation's lifetime,
// and the destination Space must have one publisher at a time.
func Publish(ctx context.Context, eng world.Engine, metadata *release.ReleaseMetadata, opts cdn_publish.Options) (*sobject.SOCheckpointInner, error) {
	checkpoint, _, err := publish(ctx, eng, metadata, opts)
	return checkpoint, err
}

// publish is Publish, also returning the head the checkpoint posted.
func publish(ctx context.Context, eng world.Engine, metadata *release.ReleaseMetadata, opts cdn_publish.Options) (*sobject.SOCheckpointInner, *bucket.ObjectRef, error) {
	// Require explicit cloud authority and a complete release before exporting.
	if eng == nil || opts.Client == nil || opts.DstSpaceID == "" || opts.OwnerKeyPem == "" || opts.CdnBaseURL == "" {
		return nil, nil, errors.New("release World, session, destination Space, owner key, and CDN URL are required")
	}
	if err := metadata.Validate(); err != nil {
		return nil, nil, err
	}
	closure, err := collectBlocks(ctx, eng, metadata)
	if err != nil {
		return nil, nil, err
	}
	head := closure.head
	opts.Log(logrus.InfoLevel, logrus.Fields{"blocks": closure.count()}, "verified release content")

	// Fetch the published pack entries.
	entries, err := cdn_publish.FetchPackEntries(ctx, opts.Client, opts.DstSpaceID)
	if err != nil {
		return nil, nil, err
	}

	// Exact pack indexes prove presence.
	// Bloom filters only narrow the search.
	remote := packfile_store.NewPackfileStore(cdn_bstore.NewAnonymousOpener(nil, opts.CdnBaseURL, opts.DstSpaceID), nil, nil)
	defer remote.Close()
	remote.UpdateManifest(entries)

	// Select missing blocks through the closure, preserving walk order.
	missing, err := closure.missing(ctx, remote)
	if err != nil {
		return nil, nil, err
	}
	opts.Log(logrus.InfoLevel, logrus.Fields{"blocks": missing.count()}, "publishing missing release content")

	// Encode packs with the standard writer and resource-scoped content
	// identity, uploading up to packUploads at once. Each chunk owns its bytes,
	// and each block's data is read back from the World as its pack fills.
	err = closure.read(ctx, eng, func(read loadBlock) error {
		// Bound the uploads in flight and count the bytes sent.
		uploads, uploadCtx := errgroup.WithContext(ctx)
		uploads.SetLimit(packUploads)
		_, missingBytes, err := missing.emit(
			uploadCtx, read, opts.DstSpaceID,
			func(ctx context.Context, chunk int, entry *packfile.PackfileEntry, data []byte) error {
				opts.Log(logrus.InfoLevel, logrus.Fields{"pack": chunk, "bytes": len(data)}, "uploading release content")
				uploads.Go(func() error {
					return cdn_publish.PushPackData(uploadCtx, opts, data, entry.GetBloomFilter())
				})
				return nil
			},
		)
		if waitErr := uploads.Wait(); err == nil {
			err = waitErr
		}
		if err == nil {
			opts.Log(logrus.InfoLevel, logrus.Fields{"bytes": missingBytes}, "uploaded missing release content")
		}
		return err
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "upload release packs")
	}

	// The signed checkpoint is the sole publication point after all packs are durable.
	checkpoint, err := cdn_publish.PostCheckpoint(ctx, opts, head)
	if err != nil {
		return nil, nil, err
	}

	// A destination that keeps every pack replaces none.
	if opts.KeepPacks {
		opts.Log(logrus.InfoLevel, nil, "kept every published pack")
		return checkpoint, head, nil
	}

	// Drop the content no longer reachable from the posted head. The release
	// is already published, so a failed pass only delays the reclaim.
	var dropped uint64
	err = closure.read(ctx, eng, func(read loadBlock) error {
		var err error
		dropped, err = closure.reclaim(ctx, opts, closure.reader(read))
		return err
	})
	fields := logrus.Fields{"dropped-bytes": dropped}
	if err != nil {
		fields[logrus.ErrorKey] = err
		opts.Log(logrus.WarnLevel, fields, "reclaim of unreferenced release content failed")
	} else {
		opts.Log(logrus.InfoLevel, fields, "reclaimed unreferenced release content")
	}
	return checkpoint, head, nil
}
