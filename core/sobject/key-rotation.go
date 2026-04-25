package sobject

import (
	"crypto/rand"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// RotateTransformKey generates a new transform key and creates grants for the
// given participants. Returns the new transform config, the grants, and the new
// epoch number. The caller (an OWNER) provides their private key for signing
// grants and the list of remaining participants (after revocation).
func RotateTransformKey(
	privKey crypto.PrivKey,
	sharedObjectID string,
	participants []*SOParticipantConfig,
	currentEpoch uint64,
	currentSeqno uint64,
) (*block_transform.Config, []*SOGrant, *SOKeyEpoch, error) {
	// Generate new random 32-byte XChaCha20-Poly1305 key.
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		return nil, nil, nil, errors.Wrap(err, "generate encryption key")
	}
	defer scrub.Scrub(encKey)

	soTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "build transform config")
	}

	// Build grants for each participant with read access.
	grantToPeerIDs := make([]string, 0, len(participants))
	for _, p := range participants {
		if CanReadState(p.GetRole()) {
			grantToPeerIDs = append(grantToPeerIDs, p.GetPeerId())
		}
	}

	grants := make([]*SOGrant, len(grantToPeerIDs))
	grantInner := &SOGrantInner{TransformConf: soTransformConf}
	for i, peerIDStr := range grantToPeerIDs {
		pid, err := peer.IDB58Decode(peerIDStr)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "participant[%d]: invalid peer id", i)
		}
		pub, err := pid.ExtractPublicKey()
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "participant[%d]: extract public key", i)
		}
		grant, err := EncryptSOGrant(privKey, pub, sharedObjectID, grantInner)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "participant[%d]: encrypt grant", i)
		}
		grants[i] = grant
	}

	nextEpoch := currentEpoch + 1
	epoch := &SOKeyEpoch{
		Epoch:      nextEpoch,
		SeqnoStart: currentSeqno + 1,
		Grants:     grants,
	}

	return soTransformConf, grants, epoch, nil
}

// FindCoveringEpoch finds the key epoch that covers the given seqno.
// Returns nil if no epoch covers the seqno.
func FindCoveringEpoch(epochs []*SOKeyEpoch, seqno uint64) *SOKeyEpoch {
	for _, ep := range epochs {
		start := ep.GetSeqnoStart()
		end := ep.GetSeqnoEnd()
		if seqno >= start && (end == 0 || seqno <= end) {
			return ep
		}
	}
	return nil
}

// CurrentEpochNumber returns the highest epoch number from the list, or 0 if empty.
func CurrentEpochNumber(epochs []*SOKeyEpoch) uint64 {
	var max uint64
	for _, ep := range epochs {
		if ep.GetEpoch() > max {
			max = ep.GetEpoch()
		}
	}
	return max
}
