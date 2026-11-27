package provider_local

import (
	"bytes"
	"context"
	"crypto/rand"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	auth_password "github.com/s4wave/spacewave/auth/method/password"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/envelope"
	"github.com/s4wave/spacewave/net/keypem"
)

// resolveEntityPrivKey derives the entity private key from an EntityCredential.
// For password credentials, uses the provider account ID as the username.
// For PEM credentials, parses the raw PEM bytes.
func resolveEntityPrivKey(providerAccountID string, cred *session.EntityCredential) (bifrost_crypto.PrivKey, error) {
	// Resolve the credential through its password or PEM representation.
	password := cred.GetPassword()
	pemPrivateKey := cred.GetPemPrivateKey()
	if password != "" {
		_, entityPriv, err := auth_password.BuildParametersWithUsernamePassword(providerAccountID, []byte(password))
		if err != nil {
			return nil, errors.Wrap(err, "derive entity key")
		}
		return entityPriv, nil
	}
	if len(pemPrivateKey) > 0 {
		privKey, err := keypem.ParsePrivKeyPem(pemPrivateKey)
		if err != nil {
			return nil, errors.Wrap(err, "parse PEM private key")
		}
		return privKey, nil
	}
	return nil, errors.New("password or pem_private_key is required")
}

// envelopeContext is the application context string for local session envelopes.
// Must be the same for build and unlock.
const envelopeContext = "local session envelope v1"

// RewrapSessionEnvelope re-wraps the session private key in a Shamir
// envelope using the current set of entity keypair public keys from the
// AccountSettings SharedObject.
//
// The envelope is written to {sessionID}/env in the session's ObjectStore.
// Threshold is 0 (any single entity keypair can recover the session key).
//
// Skipped silently when the session is locked (privkey not in memory) or
// when no entity keypairs are available.
func (a *ProviderAccount) RewrapSessionEnvelope(ctx context.Context, keypairs []*session.EntityKeypair) error {
	// Visit the Session trackers that already have a mounted Session.
	entries := a.sessions.GetKeysWithData()
	for _, entry := range entries {
		tkr := entry.Data
		prom, _ := tkr.sessionProm.GetPromise()
		if prom == nil {
			continue
		}

		// Read only an already resolved Session without waiting for a mount.
		checkCtx, checkCancel := context.WithCancel(ctx)
		checkCancel()
		sess, err := prom.Await(checkCtx)
		if err != nil || sess == nil {
			continue
		}

		// Leave locked Sessions' existing envelopes alone.
		if sess.sessionPriv == nil {
			continue
		}

		// Replace the unlocked Session's envelope with the current keypairs.
		if err := sess.rewrapEnvelope(ctx, tkr.id, keypairs); err != nil {
			return err
		}
	}
	return nil
}

// rewrapEnvelope builds and writes the Session's Shamir envelope.
func (s *Session) rewrapEnvelope(ctx context.Context, sessionID string, keypairs []*session.EntityKeypair) error {
	// Scrub the serialized Session private key when the rewrap returns.
	privPEM, err := keypem.MarshalPrivKeyPem(s.sessionPriv)
	if err != nil {
		return errors.Wrap(err, "marshal session privkey PEM")
	}
	defer scrub.Scrub(privPEM)

	// Keep the existing envelope when no entity keypairs are available.
	if len(keypairs) == 0 {
		return nil
	}

	// Decode the usable entity public keys, skipping malformed identities.
	pubKeys := make([]bifrost_crypto.PubKey, 0, len(keypairs))
	for _, kp := range keypairs {
		if kp.GetPeerId() == "" {
			continue
		}
		pub, err := session.ExtractPublicKeyFromPeerID(kp.GetPeerId())
		if err != nil {
			continue
		}
		pubKeys = append(pubKeys, pub)
	}
	if len(pubKeys) == 0 {
		return nil
	}

	// Give each entity key one independently sufficient recovery grant.
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

	// Wrap the Session private key for the configured recovery grants.
	env, err := envelope.BuildEnvelope(rand.Reader, envelopeContext, privPEM, pubKeys, config)
	if err != nil {
		return errors.Wrap(err, "build session envelope")
	}

	// Encode the envelope for the Session object store.
	envData, err := env.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal session envelope")
	}

	// Persist the replacement before reporting the rewrap complete.
	if err := session_lock.WriteEnvelope(ctx, s.objStore, sessionID, envData); err != nil {
		return errors.Wrap(err, "write session envelope")
	}
	return nil
}

// watchAndRewrapEnvelope watches the AccountSettings SO for keypair changes
// and rewraps the session envelope when they change.
func (a *ProviderAccount) watchAndRewrapEnvelope(ctx context.Context) error {
	// Resolve the account settings object whose keypairs protect the Sessions.
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}

	// Keep the settings mount alive through the watch.
	so, soRef, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer soRef()

	// Watch the settings state through the retained object.
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()

	// Rewrap Sessions whenever the settings supply a nonempty keypair set.
	var prevKeypairCount int
	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			// Rewrap the envelope when the entity keypairs change.
			if snap == nil {
				return nil
			}
			settings, err := account_settings.ReadSnapshot(ctx, snap)
			if err != nil {
				return err
			}
			keypairs := settings.GetEntityKeypairs()
			if len(keypairs) == prevKeypairCount && prevKeypairCount == 0 {
				return nil
			}
			prevKeypairCount = len(keypairs)
			return a.RewrapSessionEnvelope(ctx, keypairs)
		},
		nil,
	)
}

// UnlockSessionFromEnvelope recovers the session private key from the
// envelope using the provided entity private key.
func UnlockSessionFromEnvelope(ctx context.Context, objStore object.ObjectStore, sessionID string, entityPrivKey bifrost_crypto.PrivKey) (bifrost_crypto.PrivKey, error) {
	// Read the Session's stored recovery envelope.
	envData, err := ReadEnvelope(ctx, objStore, sessionID)
	if err != nil {
		return nil, err
	}

	// Decode the envelope's grants before attempting recovery.
	env := &envelope.Envelope{}
	if err := env.UnmarshalVT(envData); err != nil {
		return nil, errors.Wrap(err, "unmarshal envelope")
	}

	// Recover private-key bytes with the supplied entity key and scrub them.
	payload, result, err := envelope.UnlockEnvelope(envelopeContext, env, []bifrost_crypto.PrivKey{entityPrivKey})
	if err != nil {
		return nil, errors.Wrap(err, "unlock envelope")
	}
	if !result.GetSuccess() {
		return nil, errors.New("insufficient shares to unlock envelope")
	}
	defer scrub.Scrub(payload)

	// Parse the recovered Session key before its serialized bytes are scrubbed.
	recoveredKey, err := keypem.ParsePrivKeyPem(payload)
	if err != nil {
		return nil, errors.Wrap(err, "parse recovered session key")
	}
	return recoveredKey, nil
}

// HasSessionEnvelope reports whether a local session recovery envelope exists.
func HasSessionEnvelope(ctx context.Context, objStore object.ObjectStore, sessionID string) (bool, error) {
	var found bool
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			envData, attemptFound, err := tx.Get(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixEnvelope))
			if err != nil {
				return errors.Wrap(err, "read envelope")
			}
			found = attemptFound && len(envData) > 0
			return nil
		},
	)
	if err != nil {
		return false, errors.Wrap(err, "open transaction")
	}
	return found, nil
}

// ReadEnvelope reads the Shamir envelope bytes from the ObjectStore.
func ReadEnvelope(ctx context.Context, objStore object.ObjectStore, sessionID string) ([]byte, error) {
	// Copy the stored envelope out of the read transaction.
	var envData []byte
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Retain only a nonempty envelope from this transaction attempt.
			data, found, err := tx.Get(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixEnvelope))
			if err != nil {
				return errors.Wrap(err, "read envelope")
			}
			if !found || len(data) == 0 {
				return errors.New("no envelope found for session")
			}
			envData = bytes.Clone(data)
			return nil
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "open transaction")
	}
	return envData, nil
}
