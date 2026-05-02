package bldr_manifest_pack

import (
	"bytes"
	"context"
	"crypto/sha256"

	kvfile "github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// ImportManifestPack verifies and imports a manifest-pack artifact.
func ImportManifestPack(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	meta *ManifestPackMetadata,
	packBytes []byte,
) error {
	if err := meta.Validate(); err != nil {
		return err
	}
	if err := verifyPackBytes(meta, packBytes); err != nil {
		return err
	}
	if err := importPackBlocks(ctx, ws, packBytes); err != nil {
		return err
	}
	return applyManifestBundle(ctx, ws, sender, meta)
}

func verifyPackBytes(meta *ManifestPackMetadata, packBytes []byte) error {
	if uint64(len(packBytes)) != meta.GetPack().GetSizeBytes() {
		return errors.Errorf("pack size mismatch: got %d want %d", len(packBytes), meta.GetPack().GetSizeBytes())
	}
	sum := sha256.Sum256(packBytes)
	if !bytes.Equal(sum[:], meta.GetPackSha256()) {
		return errors.New("pack sha256 mismatch")
	}
	return nil
}

func importPackBlocks(ctx context.Context, ws world.WorldState, packBytes []byte) error {
	rdr, err := kvfile.BuildReader(bytes.NewReader(packBytes), uint64(len(packBytes)))
	if err != nil {
		return err
	}
	return ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		return rdr.ScanPrefixEntries(nil, func(entry *kvfile.IndexEntry, idx int) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			ref, err := parsePackBlockRef(entry)
			if err != nil {
				return errors.Wrapf(err, "pack entry %d", idx)
			}
			data, err := rdr.GetWithEntry(entry, idx)
			if err != nil {
				return errors.Wrapf(err, "read pack entry %d", idx)
			}
			_, _, err = bls.GetBucket().PutBlock(ctx, data, &block.PutOpts{
				ForceBlockRef: ref,
			})
			return err
		})
	})
}

func applyManifestBundle(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	meta *ManifestPackMetadata,
) error {
	bundle, err := readManifestBundle(ctx, ws, meta.GetManifestBundleRef())
	if err != nil {
		return err
	}
	if len(bundle.GetManifestRefs()) != len(meta.GetManifests()) {
		return errors.Errorf("manifest bundle count mismatch: got %d want %d", len(bundle.GetManifestRefs()), len(meta.GetManifests()))
	}
	obj, objOk, err := ws.GetObject(ctx, meta.GetManifests()[0].GetObjectKey())
	if err != nil {
		return err
	}
	if objOk {
		_, err = obj.SetRootRef(ctx, meta.GetManifestBundleRef())
	} else {
		_, err = ws.CreateObject(ctx, meta.GetManifests()[0].GetObjectKey(), meta.GetManifestBundleRef())
	}
	if err != nil {
		return errors.Wrap(err, "store manifest bundle")
	}
	for i, tuple := range meta.GetManifests() {
		manifestRef := bundle.GetManifestRefs()[i]
		if err := validateManifestRefMatchesTuple(manifestRef, tuple, meta.GetBuildType()); err != nil {
			return errors.Wrapf(err, "manifest %d", i)
		}
		manifestObjKey, err := bldr_manifest.NewManifestBundleEntryKey(tuple.GetObjectKey(), manifestRef.GetMeta())
		if err != nil {
			return err
		}
		_, _, err = bldr_manifest_world.SetManifest(ctx, ws, sender, manifestObjKey, manifestRef.GetManifestRef())
		if err != nil {
			return errors.Wrap(err, "store manifest")
		}
		quad := bldr_manifest_world.NewManifestQuad(tuple.GetObjectKey(), manifestObjKey, manifestRef.GetMeta().GetManifestId())
		if err := ws.SetGraphQuad(ctx, quad); err != nil {
			return errors.Wrap(err, "link bundle manifest")
		}
	}
	for _, tuple := range meta.GetManifests() {
		for _, objKey := range tuple.GetLinkObjectKeys() {
			quad := bldr_manifest_world.NewManifestQuad(objKey, tuple.GetObjectKey(), "")
			if err := ws.SetGraphQuad(ctx, quad); err != nil {
				return errors.Wrap(err, "link manifest bundle")
			}
		}
	}
	return nil
}

func readManifestBundle(
	ctx context.Context,
	ws world.WorldState,
	ref *bucket.ObjectRef,
) (*bldr_manifest.ManifestBundle, error) {
	var bundle *bldr_manifest.ManifestBundle
	_, err := world.AccessObject(ctx, ws.AccessWorldState, ref, func(bcs *block.Cursor) error {
		var err error
		bundle, err = bldr_manifest.UnmarshalManifestBundle(ctx, bcs)
		if err == nil {
			err = bundle.Validate()
		}
		return err
	})
	return bundle, err
}

func parsePackBlockRef(entry *kvfile.IndexEntry) (*block.BlockRef, error) {
	h := &hash.Hash{}
	if err := h.ParseFromB58(string(entry.GetKey())); err != nil {
		return nil, err
	}
	return &block.BlockRef{Hash: h}, nil
}
