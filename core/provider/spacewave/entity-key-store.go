package provider_spacewave

import (
	"slices"
	"sync"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/broadcast"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// EntityKeyStore tracks unlocked entity private keys in memory.
type EntityKeyStore struct {
	// bcast guards keys.
	bcast broadcast.Broadcast
	// keys contains unlocked private keys keyed by peer ID.
	keys map[peer.ID]bifrost_crypto.PrivKey
	// grace is the delay after the last retention ref drops before keys scrub.
	grace time.Duration
	// refs is the number of active retention refs.
	refs int
	// graceTimer is the pending last-ref-drop scrub timer.
	graceTimer *time.Timer
	// graceSeq invalidates timer callbacks after cancellation.
	graceSeq uint64
}

// DefaultEntityKeyStoreGrace is the default post-release key retention window.
const DefaultEntityKeyStoreGrace = 30 * time.Second

// EntityKeyStoreRef is a retention ref for unlocked entity keys.
type EntityKeyStoreRef struct {
	store *EntityKeyStore
	once  sync.Once
}

// NewEntityKeyStore creates a new EntityKeyStore.
func NewEntityKeyStore() *EntityKeyStore {
	return NewEntityKeyStoreWithGrace(DefaultEntityKeyStoreGrace)
}

// NewEntityKeyStoreWithGrace creates a new EntityKeyStore with a grace timer.
func NewEntityKeyStoreWithGrace(grace time.Duration) *EntityKeyStore {
	return &EntityKeyStore{
		keys:  make(map[peer.ID]bifrost_crypto.PrivKey),
		grace: grace,
	}
}

// Release releases a retention ref.
func (r *EntityKeyStoreRef) Release() {
	if r == nil || r.store == nil {
		return
	}
	r.once.Do(func() {
		r.store.releaseRef()
	})
}

// Retain keeps unlocked entity keys alive until the returned ref is released.
func (s *EntityKeyStore) Retain() *EntityKeyStoreRef {
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.refs++
		s.graceSeq++
		if s.graceTimer != nil {
			s.graceTimer.Stop()
			s.graceTimer = nil
		}
	})
	return &EntityKeyStoreRef{store: s}
}

// Unlock stores an unlocked private key for the given peer ID and broadcasts.
func (s *EntityKeyStore) Unlock(peerID peer.ID, privKey bifrost_crypto.PrivKey) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		scrubPrivKey(s.keys[peerID])
		s.keys[peerID] = privKey
		broadcast()
	})
}

// Lock removes the private key for the given peer ID and broadcasts.
func (s *EntityKeyStore) Lock(peerID peer.ID) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		scrubPrivKey(s.keys[peerID])
		delete(s.keys, peerID)
		broadcast()
	})
}

// LockAll removes all unlocked private keys and broadcasts.
func (s *EntityKeyStore) LockAll() {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, key := range s.keys {
			scrubPrivKey(key)
		}
		clear(s.keys)
		s.graceSeq++
		if s.graceTimer != nil {
			s.graceTimer.Stop()
			s.graceTimer = nil
		}
		broadcast()
	})
}

// IsUnlocked returns whether the given peer ID has an unlocked key.
func (s *EntityKeyStore) IsUnlocked(peerID peer.ID) bool {
	var unlocked bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, unlocked = s.keys[peerID]
	})
	return unlocked
}

// GetUnlockedCount returns the number of unlocked keys.
func (s *EntityKeyStore) GetUnlockedCount() int {
	var count int
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		count = len(s.keys)
	})
	return count
}

// GetAnyUnlockedKey returns one unlocked key using deterministic peer ordering.
func (s *EntityKeyStore) GetAnyUnlockedKey() (bifrost_crypto.PrivKey, peer.ID, bool) {
	var peerIDs []peer.ID
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for id := range s.keys {
			peerIDs = append(peerIDs, id)
		}
	})
	if len(peerIDs) == 0 {
		return nil, "", false
	}
	slices.SortFunc(peerIDs, func(a, b peer.ID) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
	var priv bifrost_crypto.PrivKey
	var ok bool
	pid := peerIDs[0]
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		priv, ok = s.keys[pid]
	})
	if !ok {
		return nil, "", false
	}
	return priv, pid, true
}

// GetUnlockedKeys returns snapshots of all unlocked private keys.
func (s *EntityKeyStore) GetUnlockedKeys() []bifrost_crypto.PrivKey {
	var keys []bifrost_crypto.PrivKey
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, key := range s.keys {
			keys = append(keys, key)
		}
	})
	return keys
}

// GetUnlockedPeerIDs returns a snapshot of all unlocked peer IDs.
func (s *EntityKeyStore) GetUnlockedPeerIDs() map[peer.ID]bool {
	result := make(map[peer.ID]bool)
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for id := range s.keys {
			result[id] = true
		}
	})
	return result
}

// SignAll signs the MultiSigActionEnvelope bytes with all unlocked keys using
// the multi-sig signing context and returns the signatures. Returns nil if no
// keys are unlocked.
func (s *EntityKeyStore) SignAll(envelope []byte) ([]*api.EntitySignature, error) {
	type keyEntry struct {
		peerID  peer.ID
		privKey bifrost_crypto.PrivKey
	}
	var entries []keyEntry
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for id, key := range s.keys {
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
func (s *EntityKeyStore) GetUnlockedKeysAndPeerIDs() ([]bifrost_crypto.PrivKey, []string) {
	var keys []bifrost_crypto.PrivKey
	var peerIDs []string
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for id, key := range s.keys {
			keys = append(keys, key)
			peerIDs = append(peerIDs, id.String())
		}
	})
	return keys, peerIDs
}

// GetBroadcast returns the broadcast for watching state changes.
func (s *EntityKeyStore) GetBroadcast() *broadcast.Broadcast {
	return &s.bcast
}

func (s *EntityKeyStore) releaseRef() {
	var expireNow bool
	var seq uint64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if s.refs == 0 {
			return
		}
		s.refs--
		if s.refs != 0 {
			return
		}
		s.graceSeq++
		seq = s.graceSeq
		if s.grace <= 0 {
			expireNow = true
			return
		}
		if s.graceTimer != nil {
			s.graceTimer.Stop()
		}
		s.graceTimer = time.AfterFunc(s.grace, func() {
			s.expireGrace(seq)
		})
	})
	if expireNow {
		s.expireGrace(seq)
	}
}

func (s *EntityKeyStore) expireGrace(seq uint64) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.refs != 0 || s.graceSeq != seq {
			return
		}
		for _, key := range s.keys {
			scrubPrivKey(key)
		}
		clear(s.keys)
		s.graceTimer = nil
		broadcast()
	})
}
