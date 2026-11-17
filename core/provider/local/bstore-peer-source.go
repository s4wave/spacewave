package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// peerSource returns the store's peer reads, which count their outcomes in
// counts.
func (t *bstoreTracker) peerSource(le *logrus.Entry, bucketID string, counts *peerReadCounts) func() block.StoreOps {
	// A store with no peers to ask reads none.
	if conf := t.nodePushConfig(); conf != nil && conf.NoPeers {
		return func() block.StoreOps { return nil }
	}
	return newPeerMissSource(
		le,
		func() block.StoreOps { return t.a.getP2PStore(bucketID) },
		func(ctx context.Context) block.StoreOps { return t.a.waitP2PStore(ctx, bucketID) },
		counts,
	)
}
