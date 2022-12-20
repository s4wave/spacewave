package identity

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
)

// LookupOrDeriveEntityKeypair attempts to resolve peer.Peer from entity keypairs.
//
// - Find all available local private keys which match the entity keypairs.
// - Allow the user to interactively derive those keypairs that we don't have.
//
// unrefDisposeDur is the duration to keep the keypair in memory.
// If unrefDisposeDur is negative, sets to the default value.
func LookupOrDeriveEntityKeypair(
	ctx context.Context,
	b bus.Bus,
	kps []*EntityKeypair,
	unrefDisposeDur time.Duration,
) ([]peer.Peer, error) {
	// Check if we already have any of them loaded.
	var lpeers []peer.Peer
	for _, selEkp := range kps {
		// Derive the keypair.
		selKp := selEkp.GetKeypair()
		peerID, err := selKp.ParsePeerID()
		if err == nil && len(peerID) == 0 {
			err = peer.ErrEmptyPeerID
		}
		if err != nil {
			return nil, errors.Wrap(err, "parse keypair peer id")
		}

		// Check if we have the private key (peer) loaded already.
		vals, valsRef, err := bus.ExecCollectValues(ctx, b, peer.NewGetPeer(peerID), nil)
		if err != nil {
			return nil, errors.Wrapf(err, "lookup peer %s", selKp.GetPeerId())
		}
		valsRef.Release()
		for _, v := range vals {
			vk, vOk := v.(peer.GetPeerValue)
			if vOk && vk != nil {
				_, err := vk.GetPrivKey(ctx)
				if err != nil {
					if err != peer.ErrNoPrivKey {
						return nil, err
					}
				} else {
					lpeers = append(lpeers, vk)
					break
				}
			}
		}
	}

	// If we don't have any loaded already, try to derive at least one.
	if len(lpeers) == 0 {
		kpv, kpvRef, err := ExDeriveEntityKeypair(ctx, b, kps, -1)
		if err != nil {
			return nil, err
		}
		lpeers = append(lpeers, kpv...)
		// the Peer objects will still be valid
		kpvRef.Release()
	}

	return lpeers, nil
}

// LookupOrDeriveKeypair attempts to resolve peer.Peer from keypairs w/o entity info.
//
// unrefDisposeDur is the duration to keep the keypair in memory.
// If unrefDisposeDur is negative, sets to the default value.
func LookupOrDeriveKeypair(
	ctx context.Context,
	b bus.Bus,
	kps []*Keypair,
	unrefDisposeDur time.Duration,
) ([]peer.Peer, error) {
	ekps := make([]*EntityKeypair, len(kps))
	for i, kp := range kps {
		ekps[i] = &EntityKeypair{Keypair: kp}
	}
	return LookupOrDeriveEntityKeypair(ctx, b, ekps, unrefDisposeDur)
}
