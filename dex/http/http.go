// Package dex_http implements data exchange via a HTTP block store.
package dex_http

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
