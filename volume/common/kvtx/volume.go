package kvtx

import (
	"context"
	"errors"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	block_gc "github.com/aperturerobotics/hydra/block/gc"
	"github.com/aperturerobotics/hydra/kvtx"
	hstore "github.com/aperturerobotics/hydra/store"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/volume"
)

// StatsFn returns storage usage statistics for a volume.
type StatsFn func(ctx context.Context) (*volume.StorageStats, error)

// Volume implements a key-value volume.
type Volume struct {
	// volumeID is the volume id
	volumeID string
	// Store is the hydra store.
	hstore.Store
	// Peer indicates the volume has a peer identity.
	peer.Peer
	// kvtxStore is the underlying kvtx store
	kvtxStore kvtx.Store
	// kvKey is the underlying kvkey
	kvKey *store_kvkey.KVKey
	// refGraph is the volume's GC reference graph.
	refGraph *block_gc.RefGraph
	// statsFn returns storage stats, may be nil.
	statsFn StatsFn
	// closeFn is the close func, may be nil
	closeFn func() error
	// deleteFn removes the backing store after Close, may be nil.
	deleteFn func() error
	// closeOnce ensures Close is idempotent.
	closeOnce sync.Once
	// closeErr stores the error from Close.
	closeErr error
}

// KvtxVolume is an interface for a volume with a kvtx store.
type KvtxVolume interface {
	// KvtxVolume extends Volume
	volume.Volume

	// GetKvtxStore returns the underlying kvtx store.
	GetKvtxStore() kvtx.Store
	// GetKvKey returns the instance of KvKey used to build keys.
	GetKvKey() *store_kvkey.KVKey
}

// NewVolume builds a new key/value volume.
//
// store /may/ optionally also be a store_kvtx.Store.
func NewVolume(
	ctx context.Context,
	storeID string,
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	conf *store_kvtx.Config,
	noGenerateKey,
	noWriteKey bool,
	statsFn StatsFn,
	closeFn func() error,
	deleteFn ...func() error,
) (*Volume, error) {
	v := &Volume{
		Store:     store_kvtx.NewKVTx(kvkey, store, conf),
		kvtxStore: store,
		kvKey:     kvkey,
		statsFn:   statsFn,
		closeFn:   closeFn,
	}
	if len(deleteFn) != 0 {
		v.deleteFn = deleteFn[0]
	}

	peerPriv, err := v.LoadPeerPriv(ctx)
	if err != nil {
		return nil, err
	}
	if peerPriv == nil {
		if noGenerateKey {
			return nil, errors.New("peer private key doesn't exist")
		}
	}

	// generates private key w/ default type if peerPriv is nil
	v.Peer, err = peer.NewPeer(peerPriv)
	if err != nil {
		return nil, err
	}

	npriv, err := v.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	if !noWriteKey && (peerPriv == nil || !npriv.Equals(peerPriv)) {
		peerPriv = npriv
		if err := v.StorePeerPriv(ctx, peerPriv); err != nil {
			return nil, err
		}
	}

	// calcuate the volume id based on the peer id
	v.volumeID = volume.NewVolumeID(storeID, v.Peer.GetPeerID())

	rg, err := block_gc.NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		return nil, err
	}
	v.refGraph = rg

	return v, nil
}

// GetID returns the computed volume id.
func (v *Volume) GetID() string {
	return v.volumeID
}

// GetPeerID returns the volume peer ID.
func (v *Volume) GetPeerID() peer.ID {
	return v.Peer.GetPeerID()
}

// GetPeer returns the Peer object.
// If withPriv=false ensure that the Peer returned does not have the private key.
func (v *Volume) GetPeer(ctx context.Context, withPriv bool) (peer.Peer, error) {
	vp := v.Peer
	if !withPriv {
		return peer.NewPeerWithPubKey(vp.GetPubKey())
	}
	return vp, nil
}

// GetKvtxStore returns the underlying kvtx store.
func (v *Volume) GetKvtxStore() kvtx.Store {
	return v.kvtxStore
}

// GetKvKey returns the instance of KvKey used to build keys.
func (v *Volume) GetKvKey() *store_kvkey.KVKey {
	return v.kvKey
}

// GetStorageStats returns storage usage statistics for the volume.
func (v *Volume) GetStorageStats(ctx context.Context) (*volume.StorageStats, error) {
	if v.statsFn != nil {
		return v.statsFn(ctx)
	}
	return &volume.StorageStats{}, nil
}

// GetRefGraph returns the volume's GC reference graph.
func (v *Volume) GetRefGraph() block_gc.RefGraphOps {
	if v.refGraph == nil {
		return nil
	}
	return v.refGraph
}

// Close closes the volume, returning any errors.
// Close is idempotent: subsequent calls return the same error.
func (v *Volume) Close() error {
	v.closeOnce.Do(func() {
		if v.refGraph != nil {
			if err := v.refGraph.Close(); err != nil {
				v.closeErr = err
				return
			}
		}
		if v.closeFn != nil {
			v.closeErr = v.closeFn()
		}
	})
	return v.closeErr
}

// Delete closes the volume and removes the backing store.
func (v *Volume) Delete() error {
	if err := v.Close(); err != nil {
		return err
	}
	if v.deleteFn != nil {
		return v.deleteFn()
	}
	return nil
}

// _ is a type assertion
var (
	_ volume.Volume = ((*Volume)(nil))
	_ KvtxVolume    = ((*Volume)(nil))
)
