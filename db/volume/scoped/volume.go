// Package volume_scoped serves a view of a volume that reaches only the data
// of one owner.
package volume_scoped

import (
	"context"

	"github.com/pkg/errors"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// Volume is a view of a volume limited to the IDs under a prefix.
//
// The view is transparent to its user: every bucket ID, object store ID and
// coordination scope gains the prefix on the way in and loses it on the way
// out, so the user cannot name data outside the prefix. The view refuses the
// peer private key, the volume's lifetime and the deletion of shared blocks.
// Blocks are shared by content address: the block hash is the capability to
// read a block, so reads and writes pass through.
type Volume struct {
	// blockStore serves the blocks.
	*blockStore
	// bucketStore serves the buckets of the view.
	*bucketStore
	// objectStore serves the object stores of the view.
	*objectStore
	// coordinator serves the coordination scopes of the view.
	*coordinator

	// inner is the underlying volume.
	inner volume.Volume
	// refGraph is the ref graph of the view, or nil if the volume has none.
	refGraph *refGraph
}

// NewVolume constructs a view of inner limited to the IDs under prefix. The
// view does not own inner: closing the view leaves inner open.
func NewVolume(inner volume.Volume, prefix string) *Volume {
	v := &Volume{
		blockStore:  &blockStore{ops: inner},
		bucketStore: &bucketStore{inner: inner, prefix: prefix},
		objectStore: &objectStore{inner: inner, prefix: prefix},
		coordinator: &coordinator{inner: inner, prefix: prefix},
		inner:       inner,
	}
	if rg := inner.GetRefGraph(); rg != nil {
		v.refGraph = &refGraph{inner: rg, prefix: prefix}
	}
	return v
}

// GetID returns the ID of the underlying volume.
func (v *Volume) GetID() string {
	return v.inner.GetID()
}

// GetPeerID returns the peer ID of the underlying volume.
func (v *Volume) GetPeerID() peer.ID {
	return v.inner.GetPeerID()
}

// GetPeer returns the peer of the underlying volume without its private key.
func (v *Volume) GetPeer(ctx context.Context, withPriv bool) (peer.Peer, error) {
	if withPriv {
		return nil, peer.ErrNoPrivKey
	}
	return v.inner.GetPeer(ctx, false)
}

// GetRefGraph returns the ref graph of the view, or nil if the volume has none.
func (v *Volume) GetRefGraph() block_gc.RefGraphOps {
	if v.refGraph == nil {
		return nil
	}
	return v.refGraph
}

// GetStorageStats returns the storage usage of the whole underlying volume.
func (v *Volume) GetStorageStats(ctx context.Context) (*volume.StorageStats, error) {
	return v.inner.GetStorageStats(ctx)
}

// Sync waits until the writes to the underlying volume are durable.
func (v *Volume) Sync(ctx context.Context) (bool, error) {
	return v.inner.Sync(ctx)
}

// LoadPeerPriv refuses to load the peer private key.
func (v *Volume) LoadPeerPriv(ctx context.Context) (crypto.PrivKey, error) {
	return nil, peer.ErrNoPrivKey
}

// StorePeerPriv refuses to overwrite the peer private key.
func (v *Volume) StorePeerPriv(ctx context.Context, privKey crypto.PrivKey) error {
	return errors.Wrap(ErrRefused, "store peer private key")
}

// Execute returns at once: the owner of the underlying volume executes it.
func (v *Volume) Execute(ctx context.Context) error {
	return nil
}

// Close leaves the underlying volume open: its owner closes it.
func (v *Volume) Close() error {
	return nil
}

// Delete refuses to delete the underlying volume.
func (v *Volume) Delete() error {
	return errors.Wrap(ErrRefused, "delete volume")
}

// _ is a type assertion
var _ volume.Volume = (*Volume)(nil)
