package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/sirupsen/logrus"
)

// peerSource returns the store's peer reads, which count their outcomes in
// counts.
func (t *bstoreTracker) peerSource(le *logrus.Entry, counts *peerReadCounts) block_store.StoreSource {
	// A store with no peers to ask reads none.
	if conf := t.nodePushConfig(); conf != nil && conf.NoPeers {
		return func() block.StoreOps { return nil }
	}

	// Read the bucket's P2P store, counting each read's outcome.
	bucketID := t.bucketID()
	return newPeerMissSource(
		le,
		func() block.StoreOps { return t.a.getP2PStore(bucketID) },
		func(ctx context.Context) block.StoreOps { return t.a.waitP2PStore(ctx, bucketID) },
		counts,
	)
}
