package psecho

import (
	"context"

	"github.com/aperturerobotics/bifrost/link"
	"github.com/aperturerobotics/bifrost/stream"
	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	dex_session "github.com/aperturerobotics/hydra/dex/session"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
)

// WithLocalOnly is a shorthand for bucket_lookup.WithLocalOnly.
var WithLocalOnly = bucket_lookup.WithLocalOnly

// getBucketLookup builds a bucket lookup handle.
// Returns the lookup, a release function, and any error.
func (c *Controller) getBucketLookup(ctx context.Context) (bucket_lookup.Lookup, func(), error) {
	lkv, _, lkRef, err := bucket_lookup.ExBuildBucketLookup(ctx, c.b, false, c.cc.GetBucketId(), nil)
	if err != nil {
		return nil, nil, err
	}
	lk, err := lkv.GetLookup(ctx)
	if err != nil {
		lkRef.Release()
		return nil, nil, err
	}
	if lk == nil {
		lkRef.Release()
		return nil, nil, nil
	}
	return lk, func() { lkRef.Release() }, nil
}

// buildIncomingRoutine constructs a routine for an incoming stream session.
func (c *Controller) buildIncomingRoutine(key sessionKey) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		return c.runIncomingStream(ctx, key)
	}, struct{}{}
}

// runIncomingStream handles an incoming sync stream.
func (c *Controller) runIncomingStream(ctx context.Context, key sessionKey) error {
	var inc *incomingStream
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		inc = c.incoming[key]
		delete(c.incoming, key)
	})
	if inc == nil {
		return nil
	}

	ms := inc.ms
	le := c.le.WithField("remote-peer", key.PeerID.String()).WithField("direction", "incoming")

	// Hold the link open while processing.
	_, lnkRef, err := c.b.AddDirective(
		link.NewEstablishLinkWithPeer(ms.GetLink().GetLocalPeer(), ms.GetPeerID()),
		nil,
	)
	if err != nil {
		ms.GetStream().Close()
		return err
	}
	defer lnkRef.Release()
	defer ms.GetStream().Close()

	sess := dex_session.NewDexSession(
		ms.GetStream(),
		int(c.cc.GetChunkSizeOrDefault()),
		c.cc.GetMaxBlockSizeOrDefault(),
	)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		reqID, ref, data, err := sess.ReceiveBlock(c.cc.GetMaxBlockSizeOrDefault())
		if err != nil {
			le.WithError(err).Debug("incoming stream ended")
			return nil
		}

		// Check if we want this block.
		var wanted bool
		c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			_, wanted = c.wantRefs[ref.MarshalString()]
		})
		if !wanted {
			_ = sess.SendCancel(reqID)
			continue
		}

		// Store the block.
		lk, rel, lerr := c.getBucketLookup(ctx)
		if lerr != nil || lk == nil {
			le.WithError(lerr).Warn("failed to get bucket lookup")
			continue
		}
		_, _, perr := lk.PutBlock(ctx, data, &block.PutOpts{
			HashType:      ref.GetHash().GetHashType(),
			ForceBlockRef: ref.Clone(),
		})
		rel()
		if perr != nil {
			le.WithError(perr).Warn("failed to store block")
			continue
		}

		le.WithField("ref", ref.MarshalString()).Debug("stored block from peer")

		// Remove from wantlist and broadcast.
		refStr := ref.MarshalString()
		c.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
			delete(c.wantRefs, refStr)
			bcast()
		})
	}
}

// buildOutgoingRoutine constructs a routine for an outgoing stream session.
func (c *Controller) buildOutgoingRoutine(key sessionKey) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		return c.runOutgoingStream(ctx, key)
	}, struct{}{}
}

// runOutgoingStream handles an outgoing sync stream to push blocks.
func (c *Controller) runOutgoingStream(ctx context.Context, key sessionKey) error {
	le := c.le.WithField("remote-peer", key.PeerID.String()).WithField("direction", "outgoing")

	// Open a stream to the remote peer.
	ms, rel, err := link.OpenStreamWithPeerEx(
		ctx, c.b, syncProtocolID,
		c.peerID, key.PeerID,
		c.cc.GetTransportId(),
		stream.OpenOpts{},
	)
	if err != nil {
		return errors.Wrap(err, "open stream")
	}
	defer rel()
	defer ms.GetStream().Close()

	sess := dex_session.NewDexSession(
		ms.GetStream(),
		int(c.cc.GetChunkSizeOrDefault()),
		c.cc.GetMaxBlockSizeOrDefault(),
	)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Pop next ref from send queue.
		var ref *block.BlockRef
		c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			rp, ok := c.remotePeers[key.PeerID]
			if !ok || len(rp.sendQueue) == 0 {
				return
			}
			ref = rp.sendQueue[0]
			rp.sendQueue = rp.sendQueue[1:]
		})
		if ref == nil {
			le.Debug("send queue empty, closing outgoing stream")
			return nil
		}

		// Look up the block data.
		lk, lkRel, lerr := c.getBucketLookup(ctx)
		if lerr != nil || lk == nil {
			le.WithError(lerr).Warn("failed to get bucket lookup for outgoing")
			continue
		}
		data, ok, lerr := lk.LookupBlock(ctx, ref, WithLocalOnly())
		lkRel()
		if lerr != nil || !ok {
			continue
		}

		// Send the block.
		le.WithField("ref", ref.MarshalString()).Debug("sending block to peer")
		if err := sess.SendBlock(c.nextRequestID(), ref, data); err != nil {
			return errors.Wrap(err, "send block")
		}
	}
}

// nextRequestID generates the next request ID.
func (c *Controller) nextRequestID() uint64 {
	var id uint64
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		c.nextNonce++
		id = c.nextNonce
	})
	return id
}
