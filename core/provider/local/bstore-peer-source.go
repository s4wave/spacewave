package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// peerSource returns the store's peer reads. A store with no peers to ask has
// no source at all, so a miss below it is proof the block is gone rather than
// a peer that may hold it once it connects.
func (t *bstoreTracker) peerSource(le *logrus.Entry, bucketID string) func() block.StoreOps {
	// A store with no peers to ask reads none.
	if conf := t.nodePushConfig(); conf != nil && conf.NoPeers {
		return nil
	}
	return newPeerMissSource(
		le,
		func() block.StoreOps { return t.a.getP2PStore(bucketID) },
		func(ctx context.Context) block.StoreOps { return t.a.waitP2PStore(ctx, bucketID) },
	)
}
