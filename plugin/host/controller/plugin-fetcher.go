package plugin_host_controller

import (
	"context"
	"sort"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/hydra/block"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

// pluginManifestFetcher tracks fetching plugin manifests.
type pluginManifestFetcher struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
	// resultPromise contains the result of the fetcher
	resultPromise *promise.PromiseContainer[*bldr_manifest.FetchManifestResponse]
}

// newPluginManifestFetcher constructs a new plugin manifest fetcher routine.
func (c *Controller) newPluginManifestFetcher(pluginID string) (keyed.Routine, *pluginManifestFetcher) {
	tr := &pluginManifestFetcher{
		c:             c,
		pluginID:      pluginID,
		resultPromise: promise.NewPromiseContainer[*bldr_manifest.FetchManifestResponse](),
	}
	return tr.execute, tr
}

// execute executes the pass tracker.
func (t *pluginManifestFetcher) execute(ctx context.Context) error {
	resultProm := promise.NewPromise[*bldr_manifest.FetchManifestResponse]()
	t.resultPromise.SetPromise(resultProm)
	resp, err := t.fetchManifest(ctx)
	resultProm.SetResult(resp, err)
	return err
}

// fetchManifest attempts to fetch the manifest.
func (t *pluginManifestFetcher) fetchManifest(ctx context.Context) (*bldr_manifest.FetchManifestResponse, error) {
	pluginID, le := t.pluginID, t.c.le
	le.Debugf("starting plugin manifest fetcher: %s", pluginID)

	// determine host plugin platform id
	hostPluginPlatformID, err := t.c.hostPluginPlatformID.Await(ctx)
	if err != nil {
		return nil, err
	}

	// build world state handle
	ws, wsRel := t.c.buildWorldState(ctx)
	defer wsRel()

	// use an empty volume ID to allow cross-volume lookup of manifest contents
	var pluginHostBucketID string
	var manifestCursorTransformConfig *block_transform.Config
	var accessManifestStorage = func(
		ctx context.Context,
		ref *bucket.ObjectRef,
		cb func(worldBaseCursor, manifestCursor *bucket_lookup.Cursor) error,
	) error {
		return ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
			// use empty volume ID to allow cross-volume lookup
			opArgs := &bucket.BucketOpArgs{}
			pluginHostBucketID = bls.GetOpArgs().GetBucketId()
			if refBucketID := ref.GetBucketId(); refBucketID != "" {
				opArgs.BucketId = refBucketID
			} else {
				opArgs.BucketId = pluginHostBucketID
			}

			manifestCursor, err := bls.FollowRefWithOpArgs(ctx, ref, opArgs)
			if err != nil {
				return err
			}
			defer manifestCursor.Release()

			manifestCursorTransformConfig = manifestCursor.GetTransformConf().Clone()
			return cb(bls, manifestCursor)
		})
	}

	// fetch the manifest for this plugin
	// wait until the plugin has been fetched
	res, err := bldr_manifest.ExFetchManifest(ctx, t.c.bus, &bldr_manifest.ManifestMeta{
		ManifestId: pluginID,
		PlatformId: hostPluginPlatformID,
	}, false)
	if err != nil {
		return nil, err
	}
	pluginManifestRef := res.GetManifestRef()
	if err := pluginManifestRef.Validate(); err != nil {
		return nil, errors.Wrap(err, "fetch plugin returned invalid manifest ref")
	}
	manifestRef := pluginManifestRef.ManifestRef
	if pluginManifestRef.GetEmpty() || manifestRef.GetEmpty() {
		return nil, errors.New("fetch plugin returned empty manifest ref")
	}

	if t.c.conf.GetDisableStoreManifest() {
		pluginManifestRef.Meta.Logger(le).Debug("skipping storing fetched manifest")
		return &bldr_manifest.FetchManifestResponse{ManifestRef: pluginManifestRef}, nil
	}

	// access manifest
	var pluginManifest *bldr_manifest.Manifest
	var manifestBucketID string
	le = pluginManifestRef.Meta.Logger(le)
	le.Debug("accessing fetched manifest")
	err = accessManifestStorage(ctx, manifestRef, func(worldCursor, manifestCursor *bucket_lookup.Cursor) error {
		_, bcs := manifestCursor.BuildTransaction(nil)
		pluginManifest, err = bldr_manifest.UnmarshalManifest(bcs)
		if err != nil {
			return err
		}
		if manifestID := pluginManifest.GetMeta().GetManifestId(); manifestID != pluginID {
			return errors.Errorf(
				"tried to fetch plugin %s but returned manifest %s",
				pluginID,
				manifestID,
			)
		}
		if err := pluginManifest.Validate(); err != nil {
			return err
		}
		manifestBucketID = manifestCursor.GetOpArgs().GetBucketId()

		// if the manifest is located in a different bucket, copy it over.
		if manifestBucketID == pluginHostBucketID {
			return nil
		}

		le.Infof("copying manifest from bucket %s to %s", manifestBucketID, pluginHostBucketID)

		writeBaseRef := manifestCursor.GetRef().Clone()
		writeBaseRef.BucketId = pluginHostBucketID
		writeCursor, err := worldCursor.FollowRef(ctx, writeBaseRef)
		if err != nil {
			if err == context.Canceled {
				return err
			}
			return errors.Wrap(err, "copy manifest: construct write cursor")
		}

		readBkt := manifestCursor.GetBucket()
		writeBkt := writeCursor.GetBucket()
		readXfrm := manifestCursor.GetTransformer()

		// To copy the object fully, we have to traverse the block graph.
		// We do this by recursively following the block refs.
		// Note that GetBlockRefCtor must be implemented for this to work properly.
		// TODO: move this code to common utility in hydra
		// TODO: handle garbage collection (set parent in PutOpts)
		type stackElem struct {
			ref  *block.BlockRef
			ctor block.Ctor
			blk  interface{}
		}
		stack := []stackElem{{ref: bcs.GetRef(), ctor: bldr_manifest.NewManifestBlock}}
		for len(stack) != 0 {
			elem := stack[len(stack)-1]
			// stack[len(stack)-1] = nil
			stack = stack[:len(stack)-1]

			blk := elem.blk
			if elem.ref != nil {
				// returns nil, false, nil if reference was empty.
				// returns nil, false, ErrNotFound if reference was not found.
				dat, found, err := readBkt.GetBlock(elem.ref)
				if err == nil && !found {
					err = block.ErrNotFound
				}
				if err != nil {
					if err == context.Canceled {
						return err
					}
					return errors.Wrapf(err, "copy manifest: fetch ref %s", elem.ref.MarshalString())
				}

				// NOTE: don't use GetBlockExists here as this goes to the "read" bucket handle.
				// if exists, existsErr := writeBkt.GetBlockExists(elem.ref); !exists || existsErr != nil {
				{
					// copy the block
					writeRef, _, err := writeBkt.PutBlock(dat, &block.PutOpts{
						HashType:      elem.ref.GetHash().GetHashType(),
						ForceBlockRef: elem.ref,
					})
					if err == nil && !writeRef.EqualsRef(elem.ref) {
						err = errors.Errorf("wrote to different ref %s", writeRef.MarshalString())
					}
					if err != nil {
						return errors.Wrapf(err, "copy manifest: write ref %s", elem.ref.MarshalString())
					}
				}
				if elem.ctor == nil {
					le.Warnf("copy manifest: ref %s: skipped block without ctor", elem.ref.MarshalString())
					continue
				}

				// construct the block
				decodeBlk := elem.ctor()

				// skip block if it has no sub-blocks or refs
				switch decodeBlk.(type) {
				case block.BlockWithRefs:
				case block.BlockWithSubBlocks:
				default:
					continue
				}

				// transform data
				dat, err = readXfrm.DecodeBlock(dat)
				if err != nil {
					return errors.Wrapf(err, "copy manifest: decode ref %s", elem.ref.MarshalString())
				}

				// unmarshal the block
				if err := decodeBlk.UnmarshalBlock(dat); err != nil {
					return errors.Wrapf(err, "copy manifest: unmarshal ref %s", elem.ref.MarshalString())
				}

				blk = decodeBlk
			} else if blk == nil {
				continue
			}

			// enqueue any sub-blocks
			if withSubBlocks, ok := blk.(block.BlockWithSubBlocks); ok {
				subBlks := withSubBlocks.GetSubBlocks()
				for _, subBlk := range subBlks {
					if subBlk != nil && !subBlk.IsNil() {
						stack = append(stack, stackElem{
							blk: subBlk,
						})
					}
				}
			}

			// if the block has no refs, continue.
			withRefs, ok := blk.(block.BlockWithRefs)
			if !ok {
				continue
			}

			blkRefs, err := withRefs.GetBlockRefs()
			if err != nil {
				return err
			}
			nextStackElems := make([]stackElem, 0, len(blkRefs))
			for refID, ref := range blkRefs {
				if ref.GetEmpty() {
					continue
				}
				nextStackElems = append(nextStackElems, stackElem{
					ref:  ref,
					ctor: withRefs.GetBlockRefCtor(refID),
				})
			}
			sort.SliceStable(nextStackElems, func(i, j int) bool {
				return nextStackElems[i].ref.LessThan(nextStackElems[j].ref)
			})
			nextStackElems = slices.CompactFunc(nextStackElems, func(a, b stackElem) bool {
				return a.ref.LessThan(b.ref)
			})
			stack = append(stack, nextStackElems...)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// adjust ref to point to the right bucket
	storedManifestRef := pluginManifestRef.CloneVT()
	storedManifestRef.ManifestRef.BucketId = pluginHostBucketID
	storedManifestRef.ManifestRef.TransformConf = manifestCursorTransformConfig

	// submit operation to update + link plugin manifest
	le.Debug("registering fetched plugin manifest")
	manifestKey := bldr_manifest.NewManifestKey(t.c.objKey, pluginManifest.GetMeta())
	err = bldr_manifest_world.ExStoreManifestOp(
		ctx,
		ws,
		t.c.peerID,
		manifestKey,
		[]string{t.c.objKey},
		storedManifestRef,
	)
	if err != nil {
		return nil, err
	}

	le.Infof("fetched stored and registered manifest for plugin: %s", t.pluginID)
	return &bldr_manifest.FetchManifestResponse{ManifestRef: storedManifestRef}, nil
}
