package provider_spacewave

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// errSessionSignerRevoked is the cause a SharedObject mount and the event
// connection end with when the signer they retained is revoked, so their
// trackers retry with the slot's current signer.
var errSessionSignerRevoked = errors.New("session signer revoked")

// SessionSigner is a retained, revocable handle to one registered Session's
// signing key and its generation.
//
// The account's session client slot binds one live SessionSigner at a time and
// gives each a generation greater than the last. The slot revokes the signer
// it held when the Session locks, logs out or unmounts, and when another
// signer replaces it. A revoked signer refuses to sign and returns a nil key,
// so every client and holder that retained it stops at once. Holders that keep
// a signer for longer than one request watch Done and resolve the slot again.
//
// A signer built for a client outside the slot, such as an entity, pairing or
// migration client, has generation zero and is never revoked. A nil
// *SessionSigner behaves as a signer with no key.
type SessionSigner struct {
	// mtx fences registered signing and request cancellation against revocation.
	mtx sync.Mutex
	// requests holds signed requests until their response bodies close, guarded by mtx.
	requests map[*context.CancelFunc]struct{}
	// sessionID is the registered Session the key belongs to, or empty for a
	// client outside the slot.
	sessionID string
	// peerID is the peer the key signs as. It stays readable after revocation
	// so logs and headers can name the peer.
	peerID peer.ID
	// gen is the slot generation the signer was bound at, or zero outside the
	// slot.
	gen uint64
	// key holds the signing key until revocation swaps it to nil.
	key atomic.Pointer[sessionSignerKey]
	// done is closed once, by revoke.
	done chan struct{}
	// revokeOnce guards closing done.
	revokeOnce sync.Once
}

// sessionSignerKey is the key a SessionSigner signs with: a private key held in
// memory, or a callback that signs where the key is held elsewhere.
type sessionSignerKey struct {
	// priv is the Ed25519 private key, or nil when sign is set.
	priv crypto.PrivKey
	// sign signs payloads when the key is held elsewhere.
	sign SigningFunc
}

// newSessionSigner builds a live signer for peerID from priv or sign. A signer
// with neither has no key and refuses to sign.
func newSessionSigner(sessionID string, peerID peer.ID, gen uint64, priv crypto.PrivKey, sign SigningFunc) *SessionSigner {
	s := &SessionSigner{
		sessionID: sessionID,
		peerID:    peerID,
		gen:       gen,
		done:      make(chan struct{}),
	}
	if priv != nil || sign != nil {
		s.key.Store(&sessionSignerKey{priv: priv, sign: sign})
	}
	return s
}

// Sign signs payload with the signer's key. It returns ErrSigningUnavailable
// when the signer has no key or was revoked.
func (s *SessionSigner) Sign(ctx context.Context, payload []byte) ([]byte, error) {
	// Fence registered key use against lock, logout and signer replacement.
	if s != nil && s.gen != 0 {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	// Refuse without a live key.
	k := s.liveKey()
	if k == nil {
		return nil, ErrSigningUnavailable
	}

	// Sign with the held key, or through the external signer.
	if k.priv != nil {
		return k.priv.Sign(payload)
	}
	return k.sign(ctx, payload)
}

// PrivKey returns the private key held in memory, or nil when the signer was
// revoked, has no key, or signs through a callback. A caller uses the key for
// the operation at hand and does not keep it past the signer's revocation.
func (s *SessionSigner) PrivKey() crypto.PrivKey {
	k := s.liveKey()
	if k == nil {
		return nil
	}
	return k.priv
}

// CanSign reports whether the signer holds a live key.
func (s *SessionSigner) CanSign() bool {
	return s.liveKey() != nil
}

// GetPeerID returns the peer the signer signs as, or empty for a nil signer.
func (s *SessionSigner) GetPeerID() peer.ID {
	if s == nil {
		return ""
	}
	return s.peerID
}

// GetSessionID returns the registered Session the signer belongs to, or empty.
func (s *SessionSigner) GetSessionID() string {
	if s == nil {
		return ""
	}
	return s.sessionID
}

// GetGeneration returns the slot generation the signer was bound at, or zero
// for a signer outside the slot.
func (s *SessionSigner) GetGeneration() uint64 {
	if s == nil {
		return 0
	}
	return s.gen
}

// Done returns a channel closed when the signer is revoked. A nil signer
// returns a nil channel, which never closes.
func (s *SessionSigner) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.done
}

// liveKey returns the signer's key, or nil when it has none or was revoked.
func (s *SessionSigner) liveKey() *sessionSignerKey {
	if s == nil {
		return nil
	}
	return s.key.Load()
}

// revoke drops the key and closes Done. Only the account's slot revokes, while
// holding the account broadcast lock. Revoking twice is a no-op.
func (s *SessionSigner) revoke() {
	// Treat an empty signer slot as already revoked.
	if s == nil {
		return
	}

	// Fence signing and cancel queued sends before lock or logout can return.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.key.Store(nil)
	s.revokeOnce.Do(func() { close(s.done) })
	for cancel := range s.requests {
		(*cancel)()
	}
	s.requests = nil
}

// bindRequestContext binds a registered Session's HTTP request to revocation.
// Revocation cancels synchronously but never joins network work. A response
// body releases the binding when closed. Standalone clients keep caller context.
func (s *SessionSigner) bindRequestContext(ctx context.Context) (context.Context, func()) {
	// Keep entity and external signers outside the registered Session lifecycle.
	if s == nil || s.gen == 0 {
		return ctx, nil
	}

	// Register request cancellation under the same fence that revokes the signer.
	requestCtx, cancel := context.WithCancel(ctx)
	s.mtx.Lock()
	if s.liveKey() == nil {
		cancel()
	} else {
		if s.requests == nil {
			s.requests = make(map[*context.CancelFunc]struct{})
		}
		s.requests[&cancel] = struct{}{}
	}
	s.mtx.Unlock()

	// Detach completed requests so revocation visits only outstanding work.
	return requestCtx, func() {
		s.mtx.Lock()
		delete(s.requests, &cancel)
		s.mtx.Unlock()
		cancel()
	}
}
