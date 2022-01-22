package kvtx

import (
	"context"
	"errors"
	"strings"

	"github.com/aperturerobotics/bifrost/keypem"
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
}

// NewVolume builds a new key/value volume.
//
// score /may/ optionally also be a store_kvtx.Store.
func NewVolume(
	ctx context.Context,
	storeID string,
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	conf *store_kvtx.Config,
	noGenerateKey bool,
) (*Volume, error) {
	v := &Volume{
		Store: store_kvtx.NewKVTx(ctx, storeID, kvkey, store, conf),
	}

	peerPriv, err := v.Store.LoadPeerPriv()
	if err != nil {
		return nil, err
	}
	if peerPriv == nil {
		if noGenerateKey {
			return nil, errors.New("peer private key doesn't exist")
		}

		peerPriv, _, err = keypem.GeneratePrivKey()
		if err != nil {
			return nil, err
		}

		if err := v.StorePeerPriv(peerPriv); err != nil {
			return nil, err
		}
	}

	v.Peer, err = peer.NewPeer(peerPriv)
	if err != nil {
		return nil, err
	}

	v.volumeID = strings.Join([]string{
		storeID,
		v.Peer.GetPeerID().Pretty(),
	}, "/")

	return v, nil
}

// GetID returns the computed volume id.
func (v *Volume) GetID() string {
	return v.volumeID
}

// Close closes the volume, returning any errors.
func (v *Volume) Close() error {
	return nil
}

// _ is a type assertion
var _ volume.Volume = ((*Volume)(nil))
