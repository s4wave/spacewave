package provider_spacewave

import (
	"bytes"
	"context"
	"sync"

	"github.com/s4wave/spacewave/net/crypto"
)

// SessionCredential is one unlocked Session generation's revocable key handle.
// A transport or signed call must never use its key after lock or logout returns.
// Revocation cancels network work and fences synchronous key use before clearing
// the key. Holders retain this handle, never its underlying private key.
type SessionCredential struct {
	// mtx fences synchronous key operations against revocation.
	mtx sync.Mutex
	// priv holds the private key until revocation, guarded by mtx.
	priv crypto.PrivKey
	// public preserves the generation's identity after revocation.
	public crypto.PubKey
	// ctx cancels work belonging to this unlocked generation.
	ctx context.Context
	// cancel revokes the generation's work without waiting for network cleanup.
	cancel context.CancelFunc
}

// newSessionCredential binds a private key to one unlocked lifetime.
func newSessionCredential(ctx context.Context, priv crypto.PrivKey) *SessionCredential {
	lifetime, cancel := context.WithCancel(ctx)
	return &SessionCredential{priv: priv, public: priv.GetPublic(), ctx: lifetime, cancel: cancel}
}

// Context returns the lifetime transport and repair work must use.
func (c *SessionCredential) Context() context.Context { return c.ctx }

// Revoked reports whether this generation has lost signing authority.
func (c *SessionCredential) Revoked() bool { return c.ctx.Err() != nil }

// Sign signs while excluding revocation, with no network work under the fence.
func (c *SessionCredential) Sign(payload []byte) ([]byte, error) {
	// Fence this synchronous signature against credential revocation.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.priv == nil || c.ctx.Err() != nil {
		return nil, ErrSigningUnavailable
	}
	return c.priv.Sign(payload)
}

// Raw serializes a live key for local credential storage.
func (c *SessionCredential) Raw() ([]byte, error) {
	// Exclude revocation while copying the local credential.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.priv == nil || c.ctx.Err() != nil {
		return nil, ErrSigningUnavailable
	}
	return c.priv.Raw()
}

// GetPublic returns the immutable public identity, including after revocation.
func (c *SessionCredential) GetPublic() crypto.PubKey { return c.public }

// Type returns the generation's key algorithm.
func (c *SessionCredential) Type() crypto.KeyType { return c.public.Type() }

// Equals compares serialized keys without nesting two credential fences.
func (c *SessionCredential) Equals(other crypto.Key) bool {
	// Snapshot each key separately so comparison cannot deadlock with revocation.
	a, err := c.Raw()
	if err != nil {
		return false
	}
	b, err := other.Raw()
	return err == nil && c.Type() == other.Type() && bytes.Equal(a, b)
}

// revoke cancels this generation and waits only for local key operations.
func (c *SessionCredential) revoke() {
	// Cancel before taking the fence so network work can unwind concurrently.
	c.cancel()

	// Fence synchronous key use before lock can return.
	c.mtx.Lock()
	c.priv = nil
	c.mtx.Unlock()
}

var _ crypto.PrivKey = (*SessionCredential)(nil)
