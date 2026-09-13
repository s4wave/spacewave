package s4wave_secret

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// WriteSecretPayloadForPeer replaces a payload only for a granted nested writer.
func WriteSecretPayloadForPeer(ctx context.Context, b bus.Bus, secret *Secret, expectedKind, writerPeerID, contentType string, value []byte) error {
	// Validate the target and authenticated caller before mounting payload state.
	if writerPeerID == "" {
		return peer.ErrEmptyPeerID
	}
	if _, err := peer.IDB58Decode(writerPeerID); err != nil {
		return err
	}
	if secret == nil || secret.GetRef() == nil {
		return ErrMissingSecretRef
	}
	if expectedKind != "" && secret.GetKind() != expectedKind {
		return ErrSecretKindMismatch
	}

	// Require write authority in the Secret's own security domain.
	so, ref, err := sobject.ExMountSharedObject(ctx, b, secret.GetRef(), false, nil)
	if err != nil {
		return err
	}
	defer ref.Release()
	host, ok := so.(sobject.InviteHost)
	if !ok {
		return ErrPayloadAccessDenied
	}
	state, err := host.GetSOHost().GetHostState(ctx)
	if err != nil {
		return err
	}
	permitted := false
	for _, participant := range state.GetConfig().GetParticipants() {
		if participant.GetPeerId() == writerPeerID && sobject.CanWriteOps(participant.GetRole()) {
			permitted = true
			break
		}
	}
	if !permitted {
		return ErrPayloadAccessDenied
	}

	// Preserve the nested object and its grants while publishing the replacement.
	previous, err := ReadSecretPayload(ctx, b, secret)
	if err != nil {
		return err
	}
	payload := newSecretPayload(value, contentType, time.Now())
	payload.Version = previous.GetVersion() + 1
	return StoreSecretPayload(ctx, b, secret.GetRef(), payload)
}
