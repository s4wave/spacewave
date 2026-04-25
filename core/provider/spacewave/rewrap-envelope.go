package provider_spacewave

import (
	"context"
	"crypto/rand"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/envelope"
	"github.com/s4wave/spacewave/net/keypem"
)

// envelopeContext is the application context string for session envelopes.
// Must be the same for build and unlock.
const envelopeContext = "spacewave session envelope v1"

// RewrapSessionEnvelope re-wraps the session private key in a Shamir
// envelope using the current set of entity keypair public keys. The
// envelope is written to {sessionID}/env in the session's ObjectStore.
//
// Threshold is 0 (any single entity keypair can recover the session key).
// This is called after keypair add/remove to keep the envelope in sync.
//
// Skipped silently when the session is locked (privkey not in memory) or
// when no entity keypairs are available.
func (a *ProviderAccount) RewrapSessionEnvelope(ctx context.Context) error {
	// Find an active, unlocked session.
	entries := a.sessions.GetKeysWithData()
	for _, entry := range entries {
		tkr := entry.Data
		prom, _ := tkr.sessionProm.GetPromise()
		if prom == nil {
			continue
		}

		// Non-blocking await: use a canceled context to check if the
		// session promise is already resolved.
		checkCtx, checkCancel := context.WithCancel(ctx)
		checkCancel()
		sess, err := prom.Await(checkCtx)
		if err != nil || sess == nil {
			continue
		}

		// Session must be unlocked.
		if sess.sessionPriv == nil {
			continue
		}

		if err := a.rewrapForSession(ctx, sess, tkr.id); err != nil {
			return err
		}
	}
	return nil
}

// rewrapForSession builds and writes a Shamir envelope for a single session.
func (a *ProviderAccount) rewrapForSession(ctx context.Context, sess *Session, sessionID string) error {
	// Marshal session private key to PEM (the payload to protect).
	privPEM, err := keypem.MarshalPrivKeyPem(sess.sessionPriv)
	if err != nil {
		return errors.Wrap(err, "marshal session privkey PEM")
	}
	defer scrub.Scrub(privPEM)

	// Get the current entity keypair public keys from account state.
	var keypairs []*session.EntityKeypair
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		keypairs = a.KeypairsSnapshot()
	})
	if len(keypairs) == 0 {
		return nil
	}

	// Parse raw pubkey bytes into crypto.PubKey objects.
	pubKeys := make([]bifrost_crypto.PubKey, 0, len(keypairs))
	for _, kp := range keypairs {
		if kp.GetPeerId() == "" {
			continue
		}
		pub, err := session.ExtractPublicKeyFromPeerID(kp.GetPeerId())
		if err != nil {
			a.le.WithError(err).Warn("skipping unparseable entity peer id for envelope")
			continue
		}
		pubKeys = append(pubKeys, pub)
	}
	if len(pubKeys) == 0 {
		return nil
	}

	// Build grant configs: one grant per keypair, each with 1 share.
	// Threshold=0 means any single share suffices (1-of-N recovery).
	grantConfigs := make([]*envelope.EnvelopeGrantConfig, len(pubKeys))
	for i := range pubKeys {
		grantConfigs[i] = &envelope.EnvelopeGrantConfig{
			ShareCount:     1,
			KeypairIndexes: []uint32{uint32(i)}, //nolint:gosec
		}
	}
	config := &envelope.EnvelopeConfig{
		Threshold:    0,
		GrantConfigs: grantConfigs,
	}

	env, err := envelope.BuildEnvelope(rand.Reader, envelopeContext, privPEM, pubKeys, config)
	if err != nil {
		return errors.Wrap(err, "build session envelope")
	}

	envData, err := env.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal session envelope")
	}

	if err := session_lock.WriteEnvelope(ctx, sess.objStore, sessionID, envData); err != nil {
		return errors.Wrap(err, "write session envelope")
	}

	a.le.Debug("re-wrapped session envelope with updated entity keypairs")
	return nil
}
