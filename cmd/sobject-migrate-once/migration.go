package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// keySet indexes in-memory local private keys by their public peer identities.
type keySet map[string]crypto.PrivKey

// add retains a key under its public identity without exposing its encoding.
func (k keySet) add(key crypto.PrivKey) error {
	// Derive the lookup identity used by configuration participants.
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return err
	}
	k[id.String()] = key
	return nil
}

// MigratedHead returns the domain-separated SHA-256 sentinel for a retained nonce.
// Its encoding is UTF-8 domain, NUL, BE32 SO-ID length, UTF-8 SO ID,
// BE32 peer-ID length, UTF-8 base58 peer ID, and BE64 unsigned nonce.
func MigratedHead(soID, peerID string, nonce uint64) []byte {
	// Frame both variable-length identities and the unsigned account nonce.
	data := []byte("spacewave/sobject/migrated-head\x00")
	data = binary.BigEndian.AppendUint32(data, uint32(len(soID)))
	data = append(data, soID...)
	data = binary.BigEndian.AppendUint32(data, uint32(len(peerID)))
	data = append(data, peerID...)
	data = binary.BigEndian.AppendUint64(data, nonce)

	// Hash the complete domain-separated frame into a current-format operation head.
	head := sha256.Sum256(data)
	return head[:]
}

// MigrateState creates an owner-signed genesis and re-signs retained root bytes.
// It leaves its input, root nonce order, inner bytes, inner sequence, and grants intact.
// All old queued operations, queued nonce tracking, and rejections are removed.
func MigrateState(soID string, state *sobject.SOState, owner, validator crypto.PrivKey) (*sobject.SOState, *sobject.SOConfigChange, error) {
	// Collapse the current configuration into one freshly owner-signed genesis.
	next := state.CloneVT()
	cfg := next.GetConfig().CloneVT()
	cfg.ConfigChainHash = nil
	cfg.ConfigChainSeqno = 0
	entry, err := sobject.BuildSOConfigChange(soID, cfg, cfg, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, owner, nil)
	if err != nil {
		return nil, nil, err
	}
	next.Config, err = sobject.VerifyConfigChange(soID, cfg, entry)
	if err != nil {
		return nil, nil, err
	}

	// Replace root heads while preserving their nonces and original order.
	root := next.GetRoot()
	for _, account := range root.GetAccountNonces() {
		account.OpHash = MigratedHead(soID, account.GetPeerId(), account.GetNonce())
	}
	root.ValidatorSignatures = nil
	if err := root.SignInnerData(validator, soID, root.GetInnerSeqno(), hash.RecommendedHashType); err != nil {
		return nil, nil, err
	}

	// Remove pending old-format envelopes and their unsigned queue tracking.
	next.Ops = nil
	next.OpRejections = nil
	next.QueuedAccountNonces = nil

	// Require state, root consensus, and the owner-signed history before persistence.
	if err := next.Validate(soID); err != nil {
		return nil, nil, err
	}
	if err := validateRoot(soID, next); err != nil {
		return nil, nil, err
	}
	if err := sobject.VerifyConfigChain(soID, []*sobject.SOConfigChange{entry}); err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(root.GetInner(), state.GetRoot().GetInner()) || root.GetInnerSeqno() != state.GetRoot().GetInnerSeqno() {
		return nil, nil, errors.New("repair changed the retained inner root")
	}
	return next, entry, nil
}

// validateRoot verifies authorized root signatures and the configured consensus.
func validateRoot(soID string, state *sobject.SOState) error {
	// Count signatures under the retained participant configuration.
	sigs, err := state.GetRoot().ValidateSignatures(soID, state.GetConfig().GetParticipants())
	if err != nil {
		return err
	}
	return sobject.CheckConsensusAcceptance(state.GetConfig().GetConsensusMode(), sigs)
}
