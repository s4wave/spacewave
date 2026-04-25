package provider_spacewave

import (
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/broadcast"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// EntityKeypairTracker tracks unlocked entity private keys in memory.
type EntityKeypairTracker struct {
	// bcast guards keys.
	bcast broadcast.Broadcast
	// keys contains unlocked private keys keyed by peer ID.
	keys map[peer.ID]bifrost_crypto.PrivKey
}

// NewEntityKeypairTracker creates a new EntityKeypairTracker.
func NewEntityKeypairTracker() *EntityKeypairTracker {
	return &EntityKeypairTracker{
		keys: make(map[peer.ID]bifrost_crypto.PrivKey),
	}
}

// Unlock stores an unlocked private key for the given peer ID and broadcasts.
func (t *EntityKeypairTracker) Unlock(peerID peer.ID, privKey bifrost_crypto.PrivKey) {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		scrubPrivKey(t.keys[peerID])
		t.keys[peerID] = privKey
		broadcast()
	})
}

// Lock removes the private key for the given peer ID and broadcasts.
func (t *EntityKeypairTracker) Lock(peerID peer.ID) {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		scrubPrivKey(t.keys[peerID])
		delete(t.keys, peerID)
		broadcast()
	})
}

// LockAll removes all unlocked private keys and broadcasts.
func (t *EntityKeypairTracker) LockAll() {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, key := range t.keys {
			scrubPrivKey(key)
		}
		clear(t.keys)
		broadcast()
	})
}

// IsUnlocked returns whether the given peer ID has an unlocked key.
func (t *EntityKeypairTracker) IsUnlocked(peerID peer.ID) bool {
	var unlocked bool
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, unlocked = t.keys[peerID]
	})
	return unlocked
}

// GetUnlockedCount returns the number of unlocked keys.
func (t *EntityKeypairTracker) GetUnlockedCount() int {
	var count int
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		count = len(t.keys)
	})
	return count
}

// GetUnlockedPeerIDs returns a snapshot of all unlocked peer IDs.
func (t *EntityKeypairTracker) GetUnlockedPeerIDs() map[peer.ID]bool {
	result := make(map[peer.ID]bool)
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for id := range t.keys {
			result[id] = true
		}
	})
	return result
}

// SignAll signs the MultiSigActionEnvelope bytes with all unlocked keys using
// the multi-sig signing context and returns the signatures. Returns nil if no
// keys are unlocked.
func (t *EntityKeypairTracker) SignAll(envelope []byte) ([]*api.EntitySignature, error) {
	type keyEntry struct {
		peerID  peer.ID
		privKey bifrost_crypto.PrivKey
	}
	var entries []keyEntry
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for id, key := range t.keys {
			entries = append(entries, keyEntry{peerID: id, privKey: key})
		}
	})
	if len(entries) == 0 {
		return nil, nil
	}
	now := timestamppb.New(time.Now().Truncate(time.Millisecond))
	payload := BuildMultiSigPayload(now, envelope)
	sigs := make([]*api.EntitySignature, 0, len(entries))
	for _, entry := range entries {
		sig, err := entry.privKey.Sign(payload)
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, &api.EntitySignature{
			PeerId:    entry.peerID.String(),
			Signature: sig,
			SignedAt:  now,
		})
	}
	return sigs, nil
}

// GetUnlockedKeysAndPeerIDs returns snapshots of all unlocked private keys and
// their corresponding peer ID strings.
func (t *EntityKeypairTracker) GetUnlockedKeysAndPeerIDs() ([]bifrost_crypto.PrivKey, []string) {
	var keys []bifrost_crypto.PrivKey
	var peerIDs []string
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for id, key := range t.keys {
			keys = append(keys, key)
			peerIDs = append(peerIDs, id.String())
		}
	})
	return keys, peerIDs
}

// GetBroadcast returns the broadcast for watching state changes.
func (t *EntityKeypairTracker) GetBroadcast() *broadcast.Broadcast {
	return &t.bcast
}
