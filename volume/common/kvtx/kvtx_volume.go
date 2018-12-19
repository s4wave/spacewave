package kvtx

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bifrost/keypem"
	"github.com/aperturerobotics/bifrost/peer"
	hstore "github.com/aperturerobotics/hydra/store"
	"github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/volume"
)

// Volume implements a key-value volume.
type Volume struct {
	// Store is the hydra store.
	hstore.Store
	// Peer indicates the volume has a peer identity.
	peer.Peer
}

// NewVolume builds a new key/value volume.
func NewVolume(
	ctx context.Context,
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	noGenerateKey bool,
) (*Volume, error) {
	v := &Volume{
		Store: kvtx.NewKVTx(ctx, kvkey, store),
	}

	peerPriv, err := v.LoadPeerPriv()
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
	}

	v.Peer, err = peer.NewPeer(peerPriv)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// GetVolumeInfo returns the basic volume information.
func (v *Volume) GetVolumeInfo() *volume.VolumeInfo {
	return &volume.VolumeInfo{}
}

// Close closes the volume, returning any errors.
func (v *Volume) Close() error {
	return nil
}

// _ is a type assertion
var _ volume.Volume = ((*Volume)(nil))
