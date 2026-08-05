package sobject

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"sync"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
)

const (
	journalNonceSize = 12
	journalKeySize   = 32

	journalCheckpointDomain = "spacewave/sharedobject-journal/checkpoint/v1"
)

func journalDefaultIdentity() []byte {
	digest := sha256.Sum256([]byte("spacewave/sharedobject-journal/test-memory-identity/v1"))
	return slices.Clone(digest[:])
}

// JournalKeyAuthority supplies the journal master key without transferring ownership.
// Implementations must derive the key from account or volume custody and never persist
// it in journal records.
type JournalKeyAuthority interface {
	JournalMasterKey(scopeID []byte) ([]byte, error)
}

// JournalKeyAuthorityFunc adapts a function to JournalKeyAuthority.
type JournalKeyAuthorityFunc func(scopeID []byte) ([]byte, error)

// JournalMasterKey implements JournalKeyAuthority.
func (f JournalKeyAuthorityFunc) JournalMasterKey(scopeID []byte) ([]byte, error) {
	if f == nil {
		return nil, ErrJournalKeyUnavailable
	}
	return f(scopeID)
}

// JournalCrypto encrypts staged journal material with domain-separated keys.
// Every staged payload is authenticated against the persisted journal identity.
type JournalCrypto struct {
	authority JournalKeyAuthority
	scopeID   []byte

	mu     sync.Mutex
	nonces map[[journalNonceSize]byte]struct{}
}

// NewJournalCrypto constructs a JournalCrypto for one journal scope.
// The scope identifier is fixed-width so derived keys cannot cross domains.
func NewJournalCrypto(scopeID []byte, authority JournalKeyAuthority) (*JournalCrypto, error) {
	if len(scopeID) != journalScopeIDSize {
		return nil, errors.Wrap(ErrJournalKeyAuthority, "journal crypto scope is invalid")
	}
	if journalIsNil(authority) {
		return nil, errors.Wrap(ErrJournalKeyAuthority, "journal key authority is required")
	}
	if authority == nil {
		return nil, errors.Wrap(ErrJournalKeyAuthority, "journal key authority is required")
	}
	return &JournalCrypto{
		authority: authority,
		scopeID:   slices.Clone(scopeID),
		nonces:    make(map[[journalNonceSize]byte]struct{}),
	}, nil
}

// SealWithIdentity encrypts canonical intent or immutable signed-envelope
// bytes and binds them to one persisted journal identity.
func (c *JournalCrypto) SealWithIdentity(kind SOJournalRecordKind, sequence uint64, key *SOMutationKey, identity, plaintext []byte) (*SOJournalEncryptedPayload, error) {
	return c.seal(kind, sequence, key, identity, plaintext)
}

func (c *JournalCrypto) seal(kind SOJournalRecordKind, sequence uint64, key *SOMutationKey, identity, plaintext []byte) (*SOJournalEncryptedPayload, error) {
	// Validate the journal scope and cipher input.
	if err := validateJournalScopeKey(c, key); err != nil {
		return nil, err
	}
	if len(identity) != sha256.Size {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "journal identity is invalid")
	}
	if err := validateJournalCipherInput(kind, key, plaintext); err != nil {
		return nil, err
	}

	// Construct the authenticated block cipher.
	block, err := c.newBlock(kind)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(ErrJournalKeyAuthority, "construct journal cipher")
	}

	// Allocate a unique nonce and authenticated metadata.
	nonce, err := c.nextNonce()
	if err != nil {
		return nil, err
	}
	aad, err := journalAAD(c.scopeID, kind, sequence, key, len(plaintext), identity)
	if err != nil {
		return nil, err
	}

	// Encrypt a scrubbed copy of the plaintext.
	staged := slices.Clone(plaintext)
	defer scrub.Scrub(staged)
	ciphertext := gcm.Seal(nil, nonce[:], staged, aad)
	return &SOJournalEncryptedPayload{
		Nonce:      slices.Clone(nonce[:]),
		Ciphertext: ciphertext,
	}, nil
}

// SealCheckpointGeneration encrypts a compact snapshot with generation-bound
// authenticated metadata. The active marker supplies the same tuple on reopen.
func (c *JournalCrypto) SealCheckpointGeneration(identity []byte, generation, nextSequence uint64, plaintext []byte) ([]byte, error) {
	// Validate checkpoint generation inputs.
	if c == nil || len(identity) != sha256.Size || len(plaintext) == 0 {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid checkpoint generation input")
	}

	// Construct the checkpoint cipher.
	block, err := c.newDomainBlock(journalCheckpointDomain)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(ErrJournalKeyAuthority, "construct checkpoint cipher")
	}

	// Allocate the nonce and bind the plaintext digest.
	nonce, err := c.nextNonce()
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(plaintext)
	aad := journalCheckpointGenerationAAD(c.scopeID, identity, generation, nextSequence, len(plaintext), digest[:])

	// Encrypt a scrubbed checkpoint copy.
	staged := slices.Clone(plaintext)
	defer scrub.Scrub(staged)
	ciphertext := gcm.Seal(nil, nonce[:], staged, aad)
	payload := &SOJournalEncryptedPayload{Nonce: slices.Clone(nonce[:]), Ciphertext: ciphertext}

	// Marshal the encrypted checkpoint payload.
	data, err := payload.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "marshal encrypted checkpoint")
	}
	return data, nil
}

// OpenCheckpointGeneration authenticates a checkpoint against the published
// identity, generation, sequence, length, and digest tuple.
func (c *JournalCrypto) OpenCheckpointGeneration(data, identity []byte, generation, nextSequence uint64, snapshotLength int, snapshotDigest []byte) ([]byte, error) {
	// Validate checkpoint metadata and decode the payload.
	if c == nil || len(identity) != sha256.Size || snapshotLength <= 0 || len(snapshotDigest) != sha256.Size {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid checkpoint generation metadata")
	}
	payload := new(SOJournalEncryptedPayload)
	if err := payload.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "decode encrypted checkpoint")
	}
	if len(payload.GetNonce()) != journalNonceSize || len(payload.GetCiphertext()) == 0 {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid encrypted checkpoint")
	}

	// Construct the checkpoint cipher for decryption.
	block, err := c.newDomainBlock(journalCheckpointDomain)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(ErrJournalKeyAuthority, "construct checkpoint cipher")
	}

	// Authenticate ciphertext length and associated metadata.
	if len(payload.GetCiphertext())-gcm.Overhead() != snapshotLength {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "checkpoint length mismatch")
	}
	aad := journalCheckpointGenerationAAD(c.scopeID, identity, generation, nextSequence, snapshotLength, snapshotDigest)
	plaintext, err := gcm.Open(nil, payload.GetNonce(), payload.GetCiphertext(), aad)
	if err != nil {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "checkpoint ciphertext authentication failed")
	}

	// Verify the recovered snapshot digest.
	digest := sha256.Sum256(plaintext)
	if !bytes.Equal(digest[:], snapshotDigest) {
		clear(plaintext)
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "checkpoint digest mismatch")
	}
	return plaintext, nil
}

// OpenWithIdentity authenticates staged ciphertext against one persisted journal identity.
func (c *JournalCrypto) OpenWithIdentity(kind SOJournalRecordKind, sequence uint64, key *SOMutationKey, identity []byte, encrypted *SOJournalEncryptedPayload) ([]byte, error) {
	return c.open(kind, sequence, key, identity, encrypted)
}

func (c *JournalCrypto) open(kind SOJournalRecordKind, sequence uint64, key *SOMutationKey, identity []byte, encrypted *SOJournalEncryptedPayload) ([]byte, error) {
	if encrypted == nil || len(encrypted.GetNonce()) != journalNonceSize || len(encrypted.GetCiphertext()) == 0 {
		return nil, errors.Wrap(ErrJournalCorrupt, "invalid encrypted journal payload")
	}
	if err := validateJournalCipherKind(kind); err != nil {
		return nil, err
	}
	if len(identity) != sha256.Size {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "journal identity is invalid")
	}
	if err := validateJournalScopeKey(c, key); err != nil {
		return nil, err
	}
	block, err := c.newBlock(kind)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(ErrJournalKeyAuthority, "construct journal cipher")
	}
	plaintextLength := len(encrypted.GetCiphertext()) - gcm.Overhead()
	if plaintextLength < 0 {
		return nil, errors.Wrap(ErrJournalCorrupt, "encrypted journal payload is shorter than authentication tag")
	}
	aad, err := journalAAD(c.scopeID, kind, sequence, key, plaintextLength, identity)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, encrypted.GetNonce(), encrypted.GetCiphertext(), aad)
	if err != nil {
		return nil, errors.Wrap(ErrJournalCorrupt, "journal ciphertext authentication failed")
	}
	return plaintext, nil
}

func (c *JournalCrypto) newBlock(kind SOJournalRecordKind) (cipher.Block, error) {
	return c.newDomainBlock(journalKeyDomain(kind))
}

func (c *JournalCrypto) newDomainBlock(domain string) (cipher.Block, error) {
	if c == nil || c.authority == nil {
		return nil, errors.Wrap(ErrJournalKeyAuthority, "journal crypto is not configured")
	}
	master, err := c.authority.JournalMasterKey(c.scopeID)
	if err != nil {
		if errors.Is(err, ErrJournalKeyUnavailable) {
			return nil, err
		}
		return nil, errors.Wrap(ErrJournalKeyAuthority, err.Error())
	}
	if len(master) != journalKeySize {
		return nil, errors.Wrap(ErrJournalKeyAuthority, "journal master key must be 32 bytes")
	}
	mac := hmac.New(sha256.New, master)
	_, _ = mac.Write([]byte(domain))
	_, _ = mac.Write(c.scopeID)
	subkey := mac.Sum(nil)
	defer scrub.Scrub(subkey)
	block, err := aes.NewCipher(subkey)
	if err != nil {
		return nil, errors.Wrap(ErrJournalKeyAuthority, "derive journal key")
	}
	return block, nil
}

func (c *JournalCrypto) nextNonce() ([journalNonceSize]byte, error) {
	var nonce [journalNonceSize]byte
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		if _, err := rand.Read(nonce[:]); err != nil {
			return nonce, errors.Wrap(ErrJournalKeyAuthority, "generate journal nonce")
		}
		if _, exists := c.nonces[nonce]; exists {
			continue
		}
		c.nonces[nonce] = struct{}{}
		return nonce, nil
	}
}

func journalCheckpointGenerationAAD(scopeID, identity []byte, generation, nextSequence uint64, snapshotLength int, snapshotDigest []byte) []byte {
	aad := make([]byte, 0, 128)
	aad = append(aad, []byte("spacewave/sharedobject-journal/checkpoint-generation-aad/v1")...)
	aad = append(aad, scopeID...)
	aad = append(aad, identity...)
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], generation)
	aad = append(aad, number[:]...)
	binary.BigEndian.PutUint64(number[:], nextSequence)
	aad = append(aad, number[:]...)
	binary.BigEndian.PutUint64(number[:], uint64(snapshotLength))
	aad = append(aad, number[:]...)
	aad = append(aad, snapshotDigest...)
	return aad
}

func validateJournalCipherInput(kind SOJournalRecordKind, key *SOMutationKey, plaintext []byte) error {
	if err := validateJournalCipherKind(kind); err != nil {
		return err
	}
	if err := key.Validate(); err != nil {
		return err
	}
	if len(plaintext) == 0 {
		return errors.Wrap(ErrJournalCorrupt, "empty staged journal payload")
	}
	return nil
}

func validateJournalScopeKey(crypto *JournalCrypto, key *SOMutationKey) error {
	if crypto == nil || len(crypto.scopeID) != journalScopeIDSize {
		return errors.Wrap(ErrJournalKeyAuthority, "journal crypto scope is unavailable")
	}
	if err := key.Validate(); err != nil {
		return err
	}
	if !bytes.Equal(crypto.scopeID, key.GetOriginScopeId()) {
		return ErrJournalScopeMismatch
	}
	return nil
}

func validateJournalCipherKind(kind SOJournalRecordKind) error {
	if kind != SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT &&
		kind != SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE {
		return errors.Wrap(ErrJournalInvalidTransition, "journal encryption kind is not staged material")
	}
	return nil
}

func journalKeyDomain(kind SOJournalRecordKind) string {
	if kind == SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT {
		return "spacewave/sharedobject-journal/canonical-intent/v1"
	}
	return "spacewave/sharedobject-journal/signed-envelope/v1"
}

func journalAAD(scopeID []byte, kind SOJournalRecordKind, sequence uint64, key *SOMutationKey, payloadLength int, identity []byte) ([]byte, error) {
	if len(scopeID) != journalScopeIDSize {
		return nil, ErrJournalKeyAuthority
	}
	if len(identity) != sha256.Size {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "journal identity is invalid")
	}
	keyBytes, err := key.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal journal AAD key")
	}
	aad := make([]byte, 0, 128+len(keyBytes))
	aad = append(aad, []byte("spacewave/sharedobject-journal/aad/v1")...)
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], uint64(JournalFormatVersion))
	aad = append(aad, number[:]...)
	binary.BigEndian.PutUint64(number[:], uint64(kind))
	aad = append(aad, number[:]...)
	binary.BigEndian.PutUint64(number[:], sequence)
	aad = append(aad, number[:]...)
	binary.BigEndian.PutUint64(number[:], uint64(payloadLength))
	aad = append(aad, number[:]...)
	aad = append(aad, scopeID...)
	aad = append(aad, []byte("journal-identity/v1")...)
	aad = append(aad, identity...)
	aad = append(aad, keyBytes...)
	return aad, nil
}
