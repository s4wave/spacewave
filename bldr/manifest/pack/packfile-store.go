package bldr_manifest_pack

import (
	"context"

	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	"github.com/s4wave/spacewave/db/block"
)

// NewPackfileStore builds a verified block store for a manifest-pack artifact.
func NewPackfileStore(
	ctx context.Context,
	meta *ManifestPackMetadata,
	opener packfile_store.Opener,
	cache packfile_store.IndexCache,
	writeback block.StoreOps,
) (*packfile_store.PackfileStore, error) {
	if err := meta.Validate(); err != nil {
		return nil, err
	}
	store := packfile_store.NewPackfileStore(opener, cache)
	store.UpdateManifest([]*packfile.PackfileEntry{meta.GetPack()})
	store.SetWriteback(ctx, writeback, 0)
	store.SetVerifyBeforeServe(true)
	return store, nil
}
