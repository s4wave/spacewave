package provider_transfer

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume"
	volume_store "github.com/s4wave/spacewave/db/volume/store"
)

// TransferKeypair copies the peer private key from source volume to target volume.
// After this call the target volume has the same peer identity as the source.
func TransferKeypair(ctx context.Context, source, target volume.Volume) error {
	sourcePeer, err := source.GetPeer(ctx, true)
	if err != nil {
		return errors.Wrap(err, "get source peer")
	}
	privKey, err := sourcePeer.GetPrivKey(ctx)
	if err != nil {
		return errors.Wrap(err, "get source private key")
	}

	targetStore, ok := target.(volume_store.Store)
	if !ok {
		return errors.New("target volume does not support key storage")
	}
	return targetStore.StorePeerPriv(ctx, privKey)
}
