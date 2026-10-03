package sobject

import (
	"crypto/rand"
	"slices"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// RotateTransformKey generates a new transform key and grants it to every
// reader in participants as the key epoch after currentEpoch. The caller, an
// owner or a voter under group control, signs the grants with privKey, passes
// the participants that remain after a revocation and records the config the
// epoch was created under.
func RotateTransformKey(
	privKey crypto.PrivKey,
	sharedObjectID string,
	participants []*SOParticipantConfig,
	currentEpoch uint64,
	configSeqno uint64,
) (*block_transform.Config, *SOKeyEpoch, error) {
	// An epoch number must never wrap into an earlier generation.
	if currentEpoch == ^uint64(0) {
		return nil, nil, errors.New("key epoch exhausted")
	}

	// Generate a new random key for the default block transform.
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		return nil, nil, errors.Wrap(err, "generate encryption key")
	}
	defer scrub.Scrub(encKey)

	// Build the transform config of the new key.
	soTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.DefaultBlockEnc,
			Key:      encKey,
		},
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "build transform config")
	}

	// Build grants for each participant with read access.
	grantToPeerIDs := make([]string, 0, len(participants))
	for _, p := range participants {
		if CanReadState(p.GetRole()) {
			grantToPeerIDs = append(grantToPeerIDs, p.GetPeerId())
		}
	}

	// Seal a grant of the config to each peer.
	grants := make([]*SOGrant, len(grantToPeerIDs))
	grantInner := &SOGrantInner{TransformConf: soTransformConf}
	for i, peerIDStr := range grantToPeerIDs {
		pid, err := peer.IDB58Decode(peerIDStr)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "participant[%d]: invalid peer id", i)
		}
		pub, err := pid.ExtractPublicKey()
		if err != nil {
			return nil, nil, errors.Wrapf(err, "participant[%d]: extract public key", i)
		}
		grant, err := EncryptSOGrant(privKey, pub, sharedObjectID, grantInner)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "participant[%d]: encrypt grant", i)
		}
		grants[i] = grant
	}

	return soTransformConf, &SOKeyEpoch{Epoch: currentEpoch + 1, Grants: grants, ConfigChainSeqno: configSeqno}, nil
}

// NeedsKeyRotation reports whether a config record after the one the latest
// of epochs was created under removed a reader, who still holds that epoch's
// key. history is the config chain in sequence order.
func NeedsKeyRotation(history []*SOConfigChange, epochs []*SOKeyEpoch) bool {
	// Start after the record the latest epoch was created under.
	var latest *SOKeyEpoch
	for _, epoch := range epochs {
		if latest == nil || epoch.GetEpoch() > latest.GetEpoch() {
			latest = epoch
		}
	}
	since := latest.GetConfigChainSeqno()

	// Look for a reader a later record no longer lets read.
	for i := 1; i < len(history); i++ {
		if history[i].GetConfigSeqno() <= since {
			continue
		}
		next := history[i].GetConfig().GetParticipants()
		for _, p := range history[i-1].GetConfig().GetParticipants() {
			if CanReadState(p.GetRole()) && !slices.ContainsFunc(next, func(n *SOParticipantConfig) bool {
				return n.GetPeerId() == p.GetPeerId() && CanReadState(n.GetRole())
			}) {
				return true
			}
		}
	}
	return false
}
