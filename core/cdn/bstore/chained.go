package cdn_bstore

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_rpc_client "github.com/s4wave/spacewave/db/block/rpc/client"
	"github.com/sirupsen/logrus"
)

// NewChainedStore reads a CDN Space first and a server's block store second,
// for a reader whose head is newer than the Space's last mirror: blocks the
// Space's packs hold come from the CDN, and the rest from server.
//
// server is reached over a connection that may drop, such as a block RPC
// client whose calls open on the current link. A read that loses its
// connection waits for the next one and is sent again, so a reconnect does
// not fail a long checkout. Writes go to server.
func NewChainedStore(ctx context.Context, le *logrus.Entry, cdn, server block.StoreOps) *block.StoreOverlay {
	return block.NewOverlay(ctx, le, block_rpc_client.ReconnectReads(server), cdn, block.OverlayMode_UPPER_READ_CACHE, 0, nil)
}
