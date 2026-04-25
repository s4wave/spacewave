package provider_local

import (
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
	"github.com/s4wave/spacewave/db/object"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/envelope"
	"github.com/s4wave/spacewave/net/keypem"
)

// resolveEntityPrivKey derives the entity private key from an EntityCredential.
// For password credentials, uses the provider account ID as the username.
// For PEM credentials, parses the raw PEM bytes.
func resolveEntityPrivKey(providerAccountID string, cred *session.EntityCredential) (bifrost_crypto.PrivKey, error) {
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
	entries := a.sessions.GetKeysWithData()
	for _, entry := range entries {
		tkr := entry.Data
		prom, _ := tkr.sessionProm.GetPromise()
		if prom == nil {
			continue
		}

		checkCtx, checkCancel := context.WithCancel(ctx)
		checkCancel()
		sess, err := prom.Await(checkCtx)
		if err != nil || sess == nil {
			continue
		}

		if sess.sessionPriv == nil {
			continue
		}

		if err := rewrapForSession(ctx, sess, tkr.id, keypairs); err != nil {
			return err
		}
	}
	return nil
}

// rewrapForSession builds and writes a Shamir envelope for a single session.
func rewrapForSession(ctx context.Context, sess *Session, sessionID string, keypairs []*session.EntityKeypair) error {
	privPEM, err := keypem.MarshalPrivKeyPem(sess.sessionPriv)
	if err != nil {
		return errors.Wrap(err, "marshal session privkey PEM")
	}
	defer scrub.Scrub(privPEM)

	if len(keypairs) == 0 {
		return nil
	}

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

	return nil
}

// watchAndRewrapEnvelope watches the AccountSettings SO for keypair changes
// and rewraps the session envelope when they change.
func (a *ProviderAccount) watchAndRewrapEnvelope(ctx context.Context) error {
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}

	so, soRef, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer soRef()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()

	var prevKeypairCount int
	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			if snap == nil {
				return nil
			}
			rootInner, err := snap.GetRootInner(ctx)
			if err != nil {
				return err
			}
			settings := &account_settings.AccountSettings{}
			if data := rootInner.GetStateData(); len(data) > 0 {
				if err := settings.UnmarshalVT(data); err != nil {
					return err
				}
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
	envData, err := ReadEnvelope(ctx, objStore, sessionID)
	if err != nil {
		return nil, err
	}

	env := &envelope.Envelope{}
	if err := env.UnmarshalVT(envData); err != nil {
		return nil, errors.Wrap(err, "unmarshal envelope")
	}

	payload, result, err := envelope.UnlockEnvelope(envelopeContext, env, []bifrost_crypto.PrivKey{entityPrivKey})
	if err != nil {
		return nil, errors.Wrap(err, "unlock envelope")
	}
	if !result.GetSuccess() {
		return nil, errors.New("insufficient shares to unlock envelope")
	}
	defer scrub.Scrub(payload)

	recoveredKey, err := keypem.ParsePrivKeyPem(payload)
	if err != nil {
		return nil, errors.Wrap(err, "parse recovered session key")
	}

	return recoveredKey, nil
}

// ReadEnvelope reads the Shamir envelope bytes from the ObjectStore.
func ReadEnvelope(ctx context.Context, objStore object.ObjectStore, sessionID string) ([]byte, error) {
	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open transaction")
	}
	defer otx.Discard()

	envData, found, err := otx.Get(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixEnvelope))
	if err != nil {
		return nil, errors.Wrap(err, "read envelope")
	}
	if !found || len(envData) == 0 {
		return nil, errors.New("no envelope found for session")
	}
	return envData, nil
}
