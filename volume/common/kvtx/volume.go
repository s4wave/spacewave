package kvtx

import (
	"context"
	"errors"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/kvtx"
	hstore "github.com/aperturerobotics/hydra/store"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/volume"
)

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
}

// KvtxVolume is an interface for a volume with a kvtx store.
type KvtxVolume interface {
	// GetKvtxStore returns the underlying kvtx store.
	GetKvtxStore() kvtx.Store
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
	noGenerateKey, noWriteKey bool,
) (*Volume, error) {
	v := &Volume{
		Store:     store_kvtx.NewKVTx(ctx, storeID, kvkey, store, conf),
		kvtxStore: store,
	}

	peerPriv, err := v.Store.LoadPeerPriv(ctx)
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

	npriv, err := v.Peer.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	if !noWriteKey && (peerPriv == nil || !npriv.Equals(peerPriv)) {
		peerPriv = npriv
		if err := v.StorePeerPriv(ctx, peerPriv); err != nil {
			return nil, err
		}
	}

	v.volumeID = strings.Join([]string{
		storeID,
		v.Peer.GetPeerID().String(),
	}, "/")

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

// Close closes the volume, returning any errors.
func (v *Volume) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ volume.Volume = ((*Volume)(nil))
	_ KvtxVolume    = ((*Volume)(nil))
)
