package space_backup

import (
	"crypto/rand"
	"crypto/sha256"
	"slices"
	"strings"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/envelope"
	"github.com/s4wave/spacewave/net/peer"
)

// envelopeContext separates backup indexes from Session and SObject envelopes.
const envelopeContext = "Spacewave SpaceBackup index v1"

// Seal encrypts every snapshot key to the account's entity public keys. Each
// recipient can independently recover it; no device identity is a recipient.
func Seal(snapshot *account_settings.SpaceBackupSnapshot, accountID, entityID string, recipients []crypto.PubKey) ([]byte, error) {
	// Keep the plaintext only for the envelope construction's lifetime.
	if len(recipients) == 0 || accountID == "" || entityID == "" || snapshot == nil {
		return nil, errors.New("backup snapshot and entity recipients are required")
	}
	data, err := snapshot.MarshalVT()
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(data)

	// Match SObject entity recovery: one share per key and threshold zero.
	grants := make([]*envelope.EnvelopeGrantConfig, len(recipients))
	for i := range recipients {
		grants[i] = &envelope.EnvelopeGrantConfig{ShareCount: 1, KeypairIndexes: []uint32{uint32(i)}} //nolint:gosec // Key counts fit in uint32.
	}
	env, err := envelope.BuildEnvelope(rand.Reader, envelopeContext, data, recipients, &envelope.EnvelopeConfig{Threshold: 0, GrantConfigs: grants})
	if err != nil {
		return nil, err
	}
	return (&account_settings.SpaceBackupIndex{ProviderAccountId: accountID, EntityId: entityID, Envelope: env}).MarshalVT()
}

// Unseal recovers the snapshot using only an account entity private key. Its
// caller need not have a device, a Session, or any of their state.
func Unseal(index *account_settings.SpaceBackupIndex, keys []crypto.PrivKey) (*account_settings.SpaceBackupSnapshot, error) {
	// Authenticate the application context before decoding root decryption keys.
	if index.GetEnvelope() == nil || len(keys) == 0 {
		return nil, errors.New("backup index and entity credential are required")
	}
	data, outcome, err := envelope.UnlockEnvelope(envelopeContext, index.GetEnvelope(), keys)
	if err != nil {
		return nil, err
	}
	if !outcome.GetSuccess() {
		return nil, errors.New("entity credential did not unlock this backup")
	}
	defer scrub.Scrub(data)

	// Return a typed snapshot without retaining the temporary plaintext buffer.
	snapshot := &account_settings.SpaceBackupSnapshot{}
	if err := snapshot.UnmarshalVT(data); err != nil {
		return nil, err
	}
	if snapshot.GetWorld().GetRootRef().GetRootRef().GetEmpty() {
		return nil, errors.New("backup index contains no World root")
	}
	return snapshot, nil
}

// recipientFingerprint binds a cached publication to its account and key set.
// Order does not matter; adding or removing a key requires a newly sealed index.
func recipientFingerprint(accountID, entityID string, recipients []crypto.PubKey) ([]byte, error) {
	// Canonicalize the public identities before hashing the cache qualifier.
	ids := make([]string, len(recipients))
	for i, public := range recipients {
		id, err := peer.IDFromPublicKey(public)
		if err != nil {
			return nil, err
		}
		ids[i] = id.String()
	}
	slices.Sort(ids)
	digest := sha256.Sum256([]byte(accountID + "\x00" + entityID + "\x00" + strings.Join(ids, "\x00")))
	return digest[:], nil
}
