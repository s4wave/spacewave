package bldr_manifest_pack

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
)

// NewHTTPPackfileStore builds a verified manifest-pack store over an HTTP range endpoint.
func NewHTTPPackfileStore(
	ctx context.Context,
	meta *ManifestPackMetadata,
	cli *http.Client,
	packURL string,
	cache packfile_store.IndexCache,
	writeback block.StoreOps,
) (*packfile_store.PackfileStore, error) {
	if packURL == "" {
		return nil, errors.New("pack url is empty")
	}
	opener := func(packID string, size int64) (*packfile_store.PackReader, error) {
		if packID != meta.GetPack().GetId() {
			return nil, errors.Errorf("unknown manifest-pack id %q", packID)
		}
		return packfile_store.NewHTTPRangeReader(cli, packURL, size, 0, nil, nil), nil
	}
	return NewPackfileStore(ctx, meta, opener, cache, writeback)
}
