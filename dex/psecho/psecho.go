// Package psecho is the pub-sub echo protocol.
//
// TODO: Constructing a proper network graph by observing route and information
// probes, as per the design I wrote in early 2019, should be the "optimal
// solution." This is a really hacky initial implementation, to test basic
// multi-device clustering.
//
// Create a "desired block waiter" which contains a reference to this
// waiting routine. Push the desired block waiter to the execute() routine,
// which ingests the request and adds the block to the wantlist.
//
// Wantlist change publishing has a time-based rate limiter, however, the
// change list will be immediately sent upon reaching a high water mark.
//
// Peers with active wantlists send an additional ping message after an
// inactivity period to indicate they still have wanted blocks.
//
// Upon encountering a unknown remote peer, either through a ping message or
// a wantlist update, a sync session is immediately triggered with the peer,
// unless the wantlist update indicates that the update contains the full
// wantlist snapshot (containing no local blocks), or the update indicates
// the wantlist is now empty. After a inactivity timeout, a remote peer will
// be marked as offline. After a longer offline timeout, a remote peer will
// be removed completely.
// /
// Buckets can optionally be configured with the psecho reconciler
// controller, which notifies the psecho controller of newly added blocks.
// This notification is used to scan the known remote wanted blocks for the
// newly added block, and if found, trigger a sync session with the peer.
//
// A sync session is triggered with a OpenStreamWithPeer, EstablishLink set of
// directives. Peers can send a variety of messages over the stream, including:
// request remote wantlist snapshot, block xmit start, block xmit chunk, refuse
// block rx.
//
// Block bodies are transmitted in 2MB chunks. If a peer receives a complete
// block from a different remote, it will send a refuse block rx message to
// cancel receiving the block from the slower peer. If a peer is already
// receiving a block from a different peer when it begins receiving a block, it
// will send a refusal message with a flag - if the flag is set, the
// transmitting peer will switch to the next block to send, if there are any
// other blocks to transmit that have not already been skipped. Otherwise, it
// will continue transmitting the data.
package psecho

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/sirupsen/logrus"
)

// MarshalBlock marshals the block to binary.
func (b *PubSubMessage) MarshalBlock() ([]byte, error) {
	return b.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (b *PubSubMessage) UnmarshalBlock(data []byte) error {
	return b.UnmarshalVT(data)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (b *PubSubMessage) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if b == nil {
		return nil
	}
	refs := b.GetWantRefs()
	if int(id) >= len(refs) {
		orefs := refs
		refs = make([]*block.BlockRef, id+1)
		copy(refs, orefs)
	}
	refs[id] = ptr
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (b *PubSubMessage) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := map[uint32]*block.BlockRef{}
	for i, x := range b.GetWantRefs() {
		if !x.GetEmpty() {
			m[uint32(i)] = x
		}
	}
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID.
func (b *PubSubMessage) GetBlockRefCtor(id uint32) block.Ctor {
	return nil
}

// LogFields attaches information about the message to a logger.
func (b *PubSubMessage) LogFields(le *logrus.Entry) *logrus.Entry {
	return le.
		WithField("clear-refs-len", len(b.GetClearRefs())).
		WithField("want-refs-len", len(b.GetWantRefs())).
		WithField("have-refs-len", len(b.GetHaveRefs())).
		WithField("want-empty", b.GetWantEmpty())
}

// _ is a type assertion
var _ block.Block = ((*PubSubMessage)(nil))
