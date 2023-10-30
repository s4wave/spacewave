package psecho

import (
	"context"
	"hash"
	"time"

	bhash "github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/bifrost/link"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/protocol"
	"github.com/aperturerobotics/bifrost/stream"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// syncProtocolID is the sync session protocol ID
var syncProtocolID = protocol.ID(ControllerID + "/sync")

// chunkSize is the chunk size in bytes
var chunkSize = 1024

// maxMessageSize is the max message size in bytes
var maxMessageSize = 2048

// blockBufferMax is the max buffer of blocks pending send
var blockBufferMax = chunkSize * 128

// maxBlockSize is the maximum incoming block size
// currently set to 10Mb
var maxBlockSize uint32 = 1e7

// remotePeer contains state for a remote peer.
type remotePeer struct {
	// c is the controller
	// immutable
	c *Controller
	// localPeer is the local peer id
	localPeer peer.ID
	// id is the peer id
	// immutable
	id peer.ID
	// wantedRefs maps string representation to ref
	// this block of fields guarded by parent mutex
	wantedRefs map[string]*block.BlockRef
	// cachedRefs maps string representation to data
	cachedRefs map[string][]byte
	// syncCtxCancel cancels the sync session
	// if nil, the sync session is not running
	syncCtxCancel context.CancelFunc
	// incSyncSessions is the number of running incoming sessions
	incSyncSessions int

	// the below fields are controlled by the sync session
	// syncBackoff controls sync session backoff
	syncBackoff backoff.BackOff
}

// newRemotePeer builds a blank remote peer
func newRemotePeer(c *Controller, localID, remoteID peer.ID) *remotePeer {
	syncBackoff := c.cc.GetSyncBackoff().Construct()
	return &remotePeer{
		id:          remoteID,
		localPeer:   localID,
		c:           c,
		wantedRefs:  make(map[string]*block.BlockRef),
		cachedRefs:  make(map[string][]byte),
		syncBackoff: syncBackoff,
	}
}

// le returns the logger
func (p *remotePeer) le() *logrus.Entry {
	return p.c.le.
		WithField("protocol-id", string(syncProtocolID)).
		WithField("remote-peer", p.id.String())
}

// triggerSyncSession starts the sync session.
func (p *remotePeer) triggerSyncSession(ctx context.Context, exitedCb func()) {
	if p.syncCtxCancel != nil {
		return
	}

	p.le().Debug("starting sync session routine")
	var subCtx context.Context
	subCtx, p.syncCtxCancel = context.WithCancel(ctx)
	go func() {
		p.executeSyncSession(subCtx)
		exitedCb()
	}()
}

// executeIncomingSyncSession executes an incoming sync session.
// the sync session exits when the remote closes the session.
// the remote closes the session when there are no blocks remaining.
func (p *remotePeer) executeIncomingSyncSession(ctx context.Context, ms link.MountedStream) error {
	ss := newSyncStream(ms.GetStream())
	var msg SyncMessage
	var rejectedBlockRecently bool
	var blockRef *block.BlockRef
	var blockBuf []byte
	var blockSize uint32
	var blockHasher hash.Hash
	le := p.le().WithField("incoming-protocol-id", ms.GetProtocolID())

	rejectBlock := func(ref *block.BlockRef) error {
		rejectedBlockRecently = true
		return ss.sendSyncMessage(&SyncMessage{
			MessageType: SyncMessageType_SyncMessageType_REFUSE_RX,
			Ref:         ref,
		})
	}
	// remain is the amount of data remaining to be received
	for {
		if err := ss.readSyncMessage(&msg); err != nil {
			return err
		}

		mt := msg.GetMessageType()
		switch mt {
		case SyncMessageType_SyncMessageType_START_XMIT:
			nBlockSize := msg.GetBlockSize()
			if nBlockSize > maxBlockSize {
				return errors.Errorf(
					"incoming block too large %d > max %d",
					nBlockSize, maxBlockSize,
				)
			}
			if blockSize != 0 && !blockRef.GetEmpty() && !rejectedBlockRecently {
				return errors.Errorf(
					"unexpected start_xmit after %d of promised %d bytes",
					len(blockBuf),
					blockSize,
				)
			}
			blockRef = msg.GetRef()
			if blockRef.GetEmpty() {
				return errors.New("empty block ref in start_xmit message")
			}
			if err := blockRef.Validate(); err != nil {
				return errors.Wrap(err, "invalid block ref")
			}
			var err error
			blockHasher, err = blockRef.GetHash().GetHashType().BuildHasher()
			if err != nil {
				return errors.Wrap(err, "build block hasher")
			}
			p.c.mtx.Lock()
			var bwaiter *desiredBlockWaiter
			for _, waiter := range p.c.waiters {
				if !waiter.ref.GetEmpty() && waiter.ref.EqualsRef(blockRef) {
					bwaiter = waiter
					break
				}
			}
			p.c.mtx.Unlock()
			if bwaiter == nil {
				// we don't want this block
				if err := rejectBlock(blockRef); err != nil {
					return err
				}
				blockSize = 0
				blockRef = nil
				continue
			}
			if cap(blockBuf) < int(nBlockSize) {
				blockBuf = make([]byte, nBlockSize)
			}
			blockBuf = blockBuf[:0]
			blockSize = nBlockSize
			rejectedBlockRecently = false
			fallthrough
		case SyncMessageType_SyncMessageType_CTNU_XMIT:
			if blockRef.GetEmpty() || cap(blockBuf) < int(blockSize) || blockSize == 0 {
				// we aren't expecting a block right now
				if rejectedBlockRecently {
					continue
				}
				return errors.New("unexpected ctnu_xmit when not receiving a block")
			}
			chunk := msg.GetChunk()
			if len(chunk) == 0 {
				return errors.Errorf("expected chunk in a %s message", mt.String())
			}
			if nlen := len(blockBuf) + len(chunk); nlen > int(blockSize) {
				return errors.Errorf("chunk received out of bounds: %d > %d", nlen, blockSize)
			}
			if _, err := blockHasher.Write(chunk); err != nil {
				return errors.Wrap(err, "hash data")
			}
			blockBuf = append(blockBuf, chunk...)
			if len(blockBuf) == int(blockSize) {
				finalBlockHash := bhash.NewHash(blockRef.GetHash().GetHashType(), blockHasher.Sum(nil))
				if !finalBlockHash.CompareHash(blockRef.GetHash()) {
					return errors.Errorf(
						"block hash mismatch: %s != expected %s",
						finalBlockHash.MarshalString(),
						blockRef.GetHash().MarshalString(),
					)
				}
				// if the size of the block is at least 60% the size of the buffer
				// then don't copy the buffer (force a re-alloc for next rx)
				// otherwise create a copy
				var fullBlock []byte
				if len(blockBuf) >= int(0.6*float32(cap(blockBuf))) {
					fullBlock = blockBuf
					blockBuf = nil
				} else {
					fullBlock = make([]byte, len(blockBuf))
					copy(fullBlock, blockBuf)
					blockBuf = blockBuf[:0]
				}
				le.
					WithField("block-ref", blockRef.MarshalString()).
					WithField("block-len", blockSize).
					Debug("received and verified block from peer")
				select {
				case <-ctx.Done():
					return ctx.Err()
				case p.c.rxBlockCh <- &rxBlock{ref: blockRef, data: fullBlock}:
				}
				blockSize = 0
				blockRef = nil
			}
		default:
			return errors.Errorf("unexpected sync message type: %s", mt.String())
		}
	}
}

// executeSyncSession executes a long-running sync session.
// the sync session exits when we have no blocks remaining to send
func (p *remotePeer) executeSyncSession(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := p.executeSyncSessionOnce(ctx); err != nil {
			if err == context.Canceled {
				return
			}
			dur := p.syncBackoff.NextBackOff()
			t := time.NewTimer(dur)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
				t.Stop()
			}
		} else {
			p.syncBackoff.Reset()
			// TODO: do we continue here?
			return
		}
	}
}

// executeSyncSessionOnce executes the sync session a single time.
func (p *remotePeer) executeSyncSessionOnce(ctx context.Context) error {
	localPeerID := p.localPeer
	remotePeerID := p.id
	le := p.le()

	// Establish link
	le.Debug("establishing link")
	_, _, lnkRef, err := bus.ExecOneOff(
		ctx,
		p.c.b,
		link.NewEstablishLinkWithPeer(localPeerID, remotePeerID),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	defer lnkRef.Release()

	// Open a stream.
	le.Debug("opening stream")
	strm, rel, err := link.OpenStreamWithPeerEx(
		ctx,
		p.c.b,
		syncProtocolID,
		localPeerID, remotePeerID,
		p.c.cc.GetTransportId(),
		stream.OpenOpts{
			Reliable:  true,
			Encrypted: true,
		},
	)
	if err != nil {
		return err
	}
	defer rel()
	defer strm.GetStream().Close()
	defer le.Debug("exiting/closing stream")

	syncStrm := newSyncStream(strm.GetStream())
	for {
		// Check if we need to offer anything.
		cachedWantedRefs := make(map[string]*block.BlockRef)
		p.c.mtx.Lock()
		cachedRefs := p.cachedRefs
		p.cachedRefs = make(map[string][]byte)
		for refStr := range cachedRefs {
			cwr, cwrOk := p.wantedRefs[refStr]
			if !cwrOk {
				delete(cachedRefs, refStr)
				continue
			}
			cachedWantedRefs[refStr] = cwr
		}
		var extraWantedRefs map[string]*block.BlockRef
		if len(cachedRefs) == 0 && len(p.wantedRefs) != 0 {
			extraWantedRefs = make(map[string]*block.BlockRef)
			for refStr, ref := range p.wantedRefs {
				extraWantedRefs[refStr] = ref
			}
		}
		p.c.mtx.Unlock()

		if len(cachedWantedRefs) == 0 {
			if len(extraWantedRefs) == 0 {
				return nil
			}

			// fetch some more blocks
			// build bucket handle
			bv, _, bvRef, err := bus.ExecOneOff(
				ctx,
				p.c.b,
				bucket_lookup.NewBuildBucketLookup(p.c.cc.GetBucketId()),
				nil,
				nil,
			)
			if err != nil {
				// TODO: handle more gracefully
				return err
			}
			lv, ok := bv.GetValue().(bucket_lookup.BuildBucketLookupValue)
			if !ok {
				bvRef.Release()
				return errors.New("build bucket lookup returned unknown value")
			}
			lk, err := lv.GetLookup(ctx)
			if err != nil {
				bvRef.Release()
				return err
			}
			var totalBuffered int
			// lookup more blocks
			for refStr, ref := range extraWantedRefs {
				if _, ok := cachedRefs[refStr]; ok {
					continue
				}

				blkDat, blkOk, err := lk.LookupBlock(ctx, ref, bucket_lookup.WithLocalOnly())
				if err != nil {
					p.c.le.
						WithField("ref", refStr).
						WithError(err).
						Warn("cannot lookup block")
					bvRef.Release()
					return err
				}
				if !blkOk {
					continue
				}
				totalBuffered += len(blkDat)
				cachedWantedRefs[refStr] = ref
				cachedRefs[refStr] = blkDat
				if totalBuffered >= blockBufferMax {
					break
				}
			}
			bvRef.Release()
			if len(cachedWantedRefs) == 0 {
				return nil
			}
		}

		for refStr, ref := range cachedWantedRefs {
			// Send the block.
			doneCh := make(chan error, 1)
			blkCtx, blkCtxCancel := context.WithCancel(ctx)
			blkDat := cachedRefs[refStr]
			xmitMsg := func() error {
				defer blkCtxCancel()
				// chunk dat by chunk size
				msg := &SyncMessage{
					MessageType: SyncMessageType_SyncMessageType_START_XMIT,
					Ref:         ref,
					BlockSize:   uint32(len(blkDat)),
				}
				for i := 0; i < len(blkDat); i += chunkSize {
					select {
					case <-blkCtx.Done():
						return blkCtx.Err()
					default:
					}
					end := i + chunkSize
					if end > len(blkDat) {
						end = len(blkDat)
					}
					msg.Chunk = blkDat[i:end]
					msg.Complete = end >= len(blkDat)
					if err := syncStrm.sendSyncMessage(msg); err != nil {
						return err
					}
					msg.MessageType = SyncMessageType_SyncMessageType_CTNU_XMIT
					msg.BlockSize = 0
				}
				return nil
			}
			go func() {
				doneCh <- xmitMsg()
			}()

			select {
			case <-ctx.Done():
				blkCtxCancel()
				_ = syncStrm.Close()
				return context.Canceled
			case err := <-doneCh:
				if err != nil {
					return err
				}
				p.c.mtx.Lock()
				delete(p.wantedRefs, refStr)
				delete(p.cachedRefs, refStr)
				p.c.mtx.Unlock()
			}
		}
	}
}

// pushWantedRefs pushes a set of wanted blocks, returning the added refs.
func (p *remotePeer) pushWantedRefs(refs []*block.BlockRef) []*block.BlockRef {
	var added []*block.BlockRef
	for _, r := range refs {
		if r.GetEmpty() {
			continue
		}
		if err := r.Validate(); err != nil {
			continue
		}
		rid := r.MarshalString()
		if _, ok := p.wantedRefs[rid]; !ok {
			p.wantedRefs[rid] = r
			added = append(added, r)
		}
	}
	return added
}
