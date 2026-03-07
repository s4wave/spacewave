package volume_controller

import (
	"context"

	block_gc "github.com/aperturerobotics/hydra/block/gc"
	"github.com/aperturerobotics/hydra/volume"
)

// NodeUnknownLegacy is the IRI for legacy blocks with unknown provenance.
const NodeUnknownLegacy = "unknown"

// bootstrapGC initializes the GC ref graph for an existing volume.
// Called when the ref graph is empty (first GC-enabled startup).
func (c *Controller) bootstrapGC(
	ctx context.Context,
	rg *block_gc.RefGraph,
	vol volume.Volume,
	mode GCBootstrapMode,
) error {
	if mode == GCBootstrapMode_GC_IGNORE {
		c.le.Info("gc bootstrap: GC_IGNORE mode, skipping")
		return nil
	}

	// Enumerate all buckets in the volume.
	buckets, err := vol.ListBucketInfo(ctx, nil)
	if err != nil {
		c.le.WithError(err).Warn("gc bootstrap: unable to list buckets, skipping")
		return nil
	}

	swept := 0
	for _, bi := range buckets {
		if err := ctx.Err(); err != nil {
			return err
		}

		bucketID := bi.GetConfig().GetId()
		if bucketID == "" {
			continue
		}

		// TODO: enumerate blocks per bucket.
		// The current Store interface does not expose a block iteration API.
		// For now, log the bucket and skip block enumeration.
		// When a block iteration API is added, iterate blocks here and:
		// - GC_LEGACY: rg.AddRef(ctx, NodeUnknownLegacy, block_gc.BlockIRI(ref))
		// - GC_EXISTING: rg.AddRef(ctx, block_gc.NodeUnreferenced, block_gc.BlockIRI(ref))
		swept++
	}

	switch mode {
	case GCBootstrapMode_GC_LEGACY:
		c.le.WithField("buckets", swept).Info("gc bootstrap: GC_LEGACY mode (no block iteration API, buckets enumerated)")
	case GCBootstrapMode_GC_EXISTING:
		c.le.WithField("buckets", swept).Info("gc bootstrap: GC_EXISTING mode (no block iteration API, buckets enumerated)")
	}

	return nil
}
