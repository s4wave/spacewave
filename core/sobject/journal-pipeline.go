package sobject

import (
	"bytes"
	"crypto/sha256"
	"slices"
	"sync"

	"github.com/pkg/errors"
)

// JournalPipeline couples durable framing with the deterministic reducer.
type JournalPipeline struct {
	mu             sync.Mutex
	journal        *journal
	reducer        *JournalReducer
	crypto         *JournalCrypto
	verifier       JournalReceiptVerifier
	lookupVerifier JournalLookupVerifier
}

// JournalReceiptVerifier authenticates opaque terminal receipt bytes and their
// recorded config/consensus/signature-set identity before durable append.
type JournalReceiptVerifier interface {
	VerifyJournalReceipt(receipt *SOJournalReceipt, version *SOJournalVersionTuple) error
}

// JournalReceiptVerifierFunc adapts a receipt verification function.
type JournalReceiptVerifierFunc func(*SOJournalReceipt, *SOJournalVersionTuple) error

// VerifyJournalReceipt implements JournalReceiptVerifier.
func (f JournalReceiptVerifierFunc) VerifyJournalReceipt(receipt *SOJournalReceipt, version *SOJournalVersionTuple) error {
	if f == nil {
		return ErrJournalReceiptVerifierRequired
	}
	return f(receipt, version)
}

// JournalLookupVerifier authenticates every opaque receipt lookup response.
type JournalLookupVerifier interface {
	VerifyJournalLookup(lookup *SOJournalLookup, version *SOJournalVersionTuple) error
}

// JournalLookupVerifierFunc adapts a lookup verification function.
type JournalLookupVerifierFunc func(*SOJournalLookup, *SOJournalVersionTuple) error

// VerifyJournalLookup implements JournalLookupVerifier.
func (f JournalLookupVerifierFunc) VerifyJournalLookup(lookup *SOJournalLookup, version *SOJournalVersionTuple) error {
	if f == nil {
		return ErrJournalLookupVerifierRequired
	}
	return f(lookup, version)
}

// OpenJournalPipeline opens and replays a journal before admitting new transitions.
// A writable pipeline requires both at-rest crypto and receipt authority.
func OpenJournalPipeline(storage JournalStorage) (*JournalPipeline, error) {
	if journalIsNil(storage) {
		return nil, ErrJournalStorageRequired
	}
	return nil, ErrJournalKeyUnavailable
}

// OpenJournalPipelineWithCrypto authenticates retained encrypted stages,
// terminal receipts, and lookup responses before returning a writable pipeline.
func OpenJournalPipelineWithCrypto(storage JournalStorage, crypto *JournalCrypto, verifier JournalReceiptVerifier, lookupVerifier JournalLookupVerifier) (*JournalPipeline, error) {
	if journalIsNil(storage) {
		return nil, ErrJournalStorageRequired
	}
	if crypto == nil {
		return nil, errors.Wrap(ErrJournalKeyUnavailable, "journal crypto is required")
	}
	if journalIsNil(verifier) {
		return nil, ErrJournalReceiptVerifierRequired
	}
	if journalIsNil(lookupVerifier) {
		return nil, ErrJournalLookupVerifierRequired
	}
	return openJournalPipeline(storage, crypto, verifier, lookupVerifier)
}

func openJournalPipeline(storage JournalStorage, crypto *JournalCrypto, verifier JournalReceiptVerifier, lookupVerifier JournalLookupVerifier) (*JournalPipeline, error) {
	journal, err := openJournal(storage, crypto)
	if err != nil {
		return nil, err
	}
	records := journal.replay()
	if err := authenticateJournalRecords(records, crypto, journal.writer.identity); err != nil {
		return nil, err
	}
	if err := authenticateJournalSnapshots(journal.writer.reducer.Snapshot(), crypto, journal.writer.identity); err != nil {
		return nil, err
	}
	if err := verifyJournalReceipts(records, verifier); err != nil {
		return nil, err
	}
	if err := verifyJournalLookups(records, lookupVerifier); err != nil {
		return nil, err
	}
	if err := verifyJournalSnapshots(journal.writer.reducer.Snapshot(), verifier, lookupVerifier); err != nil {
		return nil, err
	}
	if err := journal.writer.activatePending(); err != nil {
		return nil, err
	}
	return &JournalPipeline{journal: journal, reducer: journal.writer.reducer, crypto: crypto, verifier: verifier, lookupVerifier: lookupVerifier}, nil
}

func verifyJournalLookups(records []*SOJournalRecord, verifier JournalLookupVerifier) error {
	if verifier == nil {
		return ErrJournalLookupVerifierRequired
	}
	for _, record := range records {
		if record != nil && record.GetKind() == SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP {
			if err := verifyJournalLookup(verifier, record.GetLookup(), record.GetVersion()); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyJournalLookup(verifier JournalLookupVerifier, lookup *SOJournalLookup, version *SOJournalVersionTuple) error {
	if verifier == nil {
		return ErrJournalLookupVerifierRequired
	}
	if err := verifier.VerifyJournalLookup(lookup, version); err != nil {
		return errors.Wrap(ErrJournalLookupVerification, err.Error())
	}
	return nil
}

func verifyJournalReceipts(records []*SOJournalRecord, verifier JournalReceiptVerifier) error {
	if verifier == nil {
		return ErrJournalReceiptVerifierRequired
	}
	for _, record := range records {
		if receipt := journalRecordReceipt(record); receipt != nil {
			if err := verifyJournalReceipt(verifier, receipt, record.GetVersion()); err != nil {
				return err
			}
		}
	}
	return nil
}

func journalRecordReceipt(record *SOJournalRecord) *SOJournalReceipt {
	if record == nil {
		return nil
	}
	if record.GetKind() == SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT {
		return record.GetReceipt()
	}
	if record.GetKind() == SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP {
		lookup := record.GetLookup()
		if lookup != nil && (lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_ACCEPTED || lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_REJECTED) {
			return lookup.GetReceipt()
		}
	}
	return nil
}

func verifyJournalSnapshots(snapshots []*JournalAttemptSnapshot, verifier JournalReceiptVerifier, lookupVerifier JournalLookupVerifier) error {
	if verifier == nil {
		return ErrJournalReceiptVerifierRequired
	}
	if lookupVerifier == nil {
		return ErrJournalLookupVerifierRequired
	}
	for _, attempt := range snapshots {
		if attempt == nil {
			return ErrJournalCheckpointCorrupt
		}
		if attempt.Receipt != nil {
			if err := verifyJournalReceipt(verifier, attempt.Receipt, attempt.Version); err != nil {
				return err
			}
		}
		if attempt.Lookup != nil {
			if err := verifyJournalLookup(lookupVerifier, attempt.Lookup, attempt.Version); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyJournalReceipt(verifier JournalReceiptVerifier, receipt *SOJournalReceipt, version *SOJournalVersionTuple) error {
	if verifier == nil {
		return ErrJournalReceiptVerifierRequired
	}
	if err := verifier.VerifyJournalReceipt(receipt, version); err != nil {
		return errors.Wrap(ErrJournalReceiptVerification, err.Error())
	}
	return nil
}

func authenticateJournalRecords(records []*SOJournalRecord, crypto *JournalCrypto, identity []byte) error {
	for _, record := range records {
		switch record.GetKind() {
		case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT:
			if err := authenticateJournalIntent(record, crypto, identity); err != nil {
				return err
			}
		case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE:
			if err := authenticateJournalEnvelope(record, crypto, identity); err != nil {
				return err
			}
		}
	}
	return nil
}

func authenticateJournalIntent(record *SOJournalRecord, crypto *JournalCrypto, identity []byte) error {
	if crypto == nil {
		return errors.Wrap(ErrJournalKeyUnavailable, "journal intent requires at-rest key")
	}
	plaintext, err := crypto.OpenWithIdentity(record.GetKind(), record.GetSequence(), record.GetKey(), identity, record.GetIntent())
	if err != nil {
		return err
	}
	defer clear(plaintext)
	intent := new(SOJournalIntent)
	if err := intent.UnmarshalVT(plaintext); err != nil {
		return errors.Wrap(ErrJournalCorrupt, "decode retained journal intent")
	}
	if !intent.GetKey().EqualExact(record.GetKey()) ||
		!intent.GetLineage().GetRootKey().EqualExact(record.GetLineage().GetRootKey()) ||
		!sameOptionalKey(intent.GetLineage().GetSupersedes(), record.GetLineage().GetSupersedes()) ||
		!intent.GetVersion().EqualVT(record.GetVersion()) {
		return errors.Wrap(ErrJournalCorrupt, "retained journal intent identity mismatch")
	}
	return nil
}

func authenticateJournalEnvelope(record *SOJournalRecord, crypto *JournalCrypto, identity []byte) error {
	if crypto == nil {
		return errors.Wrap(ErrJournalKeyUnavailable, "journal envelope requires at-rest key")
	}
	plaintext, err := crypto.OpenWithIdentity(record.GetKind(), record.GetSequence(), record.GetKey(), identity, record.GetEnvelope())
	if err != nil {
		return err
	}
	defer clear(plaintext)
	digest := sha256.Sum256(plaintext)
	if !bytes.Equal(digest[:], record.GetEnvelopeDigest()) {
		return errors.Wrap(ErrJournalCorrupt, "retained envelope digest mismatch")
	}
	return nil
}
func authenticateJournalSnapshots(snapshots []*JournalAttemptSnapshot, crypto *JournalCrypto, identity []byte) error {
	if crypto == nil {
		return errors.Wrap(ErrJournalKeyUnavailable, "compact journal stages require at-rest key")
	}
	if len(identity) != sha256.Size {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "compact journal identity is invalid")
	}
	for _, attempt := range snapshots {
		if attempt == nil {
			return ErrJournalCheckpointCorrupt
		}
		intentPlaintext, err := crypto.OpenWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, attempt.IntentSequence, attempt.Key, identity, attempt.Intent)
		if err != nil {
			return err
		}
		intent := new(SOJournalIntent)
		if err := intent.UnmarshalVT(intentPlaintext); err != nil {
			clear(intentPlaintext)
			return errors.Wrap(ErrJournalCorrupt, "decode compact journal intent")
		}
		clear(intentPlaintext)
		if !intent.GetKey().EqualExact(attempt.Key) ||
			!intent.GetLineage().GetRootKey().EqualExact(attempt.Lineage.GetRootKey()) ||
			!sameOptionalKey(intent.GetLineage().GetSupersedes(), attempt.Lineage.GetSupersedes()) ||
			!intent.GetVersion().EqualVT(attempt.Version) {
			return errors.Wrap(ErrJournalCorrupt, "compact journal intent identity mismatch")
		}
		if attempt.Envelope != nil {
			envelope, err := crypto.OpenWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE, attempt.EnvelopeSequence, attempt.Key, identity, attempt.Envelope)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(envelope)
			clear(envelope)
			if !bytes.Equal(digest[:], attempt.EnvelopeDigest) {
				return errors.Wrap(ErrJournalCorrupt, "compact journal envelope digest mismatch")
			}
		}
	}
	return nil
}

// appendRecord validates a transition against the live reducer before durable append.
// It is intentionally package-private: production callers must use a state-specific
// method that derives the immutable attempt identity under the pipeline lock.
func (p *JournalPipeline) appendRecord(record *SOJournalRecord) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.appendLocked(record)
}

func (p *JournalPipeline) appendLocked(record *SOJournalRecord) error {
	if p == nil || p.journal == nil || p.journal.writer == nil {
		return ErrJournalStorageRequired
	}
	if record == nil {
		return ErrJournalCorrupt
	}
	prepared := record.CloneVT()
	prepared.FormatVersion = JournalFormatVersion
	expectedSequence := p.journal.nextSequence()
	if expectedSequence == 0 {
		return ErrJournalStorageRequired
	}
	if prepared.Sequence == 0 {
		prepared.Sequence = expectedSequence
	} else if prepared.Sequence != expectedSequence {
		return errors.Wrap(ErrJournalCorrupt, "journal sequence is not writer-owned")
	}
	if p.crypto != nil {
		if err := authenticateJournalRecords([]*SOJournalRecord{prepared}, p.crypto, p.journal.writer.identity); err != nil {
			return err
		}
	}
	if receipt := journalRecordReceipt(prepared); receipt != nil {
		if err := verifyJournalReceipt(p.verifier, receipt, prepared.GetVersion()); err != nil {
			return err
		}
	}
	if prepared.GetKind() == SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP {
		if err := verifyJournalLookup(p.lookupVerifier, prepared.GetLookup(), prepared.GetVersion()); err != nil {
			return err
		}
	}
	return p.journal.append(prepared)
}

func (p *JournalPipeline) attemptForAppendLocked(key *SOMutationKey) (*JournalAttemptSnapshot, error) {
	if p == nil || p.reducer == nil {
		return nil, ErrJournalStorageRequired
	}
	return p.reducer.Attempt(key)
}

// AppendReceipt records verified terminal remote evidence for the live attempt.
func (p *JournalPipeline) AppendReceipt(key *SOMutationKey, receipt *SOJournalReceipt) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, err := p.attemptForAppendLocked(key)
	if err != nil {
		return err
	}
	return p.appendLocked(NewJournalReceiptRecord(attempt.Key, attempt.Lineage, attempt.Version, receipt))
}

// AppendReceiptLookup records an exact-key remote lookup for the live attempt.
func (p *JournalPipeline) AppendReceiptLookup(key *SOMutationKey, lookup *SOJournalLookup) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, err := p.attemptForAppendLocked(key)
	if err != nil {
		return err
	}
	return p.appendLocked(NewJournalReceiptLookupRecord(attempt.Key, attempt.Lineage, attempt.Version, lookup))
}

// AppendLookup is the concise alias for AppendReceiptLookup.
func (p *JournalPipeline) AppendLookup(key *SOMutationKey, lookup *SOJournalLookup) error {
	return p.AppendReceiptLookup(key, lookup)
}

// AppendResendAuthorization records an authoritative no-record lookup result.
func (p *JournalPipeline) AppendResendAuthorization(key *SOMutationKey) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, err := p.attemptForAppendLocked(key)
	if err != nil {
		return err
	}
	return p.appendLocked(NewJournalResendAuthorizedRecord(attempt.Key, attempt.Lineage, attempt.Version))
}

// AppendAcknowledgement records an acknowledgement bound to the live receipt.
func (p *JournalPipeline) AppendAcknowledgement(key *SOMutationKey, acknowledgement *SOJournalAcknowledgement) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, err := p.attemptForAppendLocked(key)
	if err != nil {
		return err
	}
	return p.appendLocked(NewJournalAcknowledgementRecord(attempt.Key, attempt.Lineage, attempt.Version, acknowledgement))
}

// AppendProjection records the body projection bound to the live receipt root.
func (p *JournalPipeline) AppendProjection(key *SOMutationKey, projection *SOJournalProjection) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, err := p.attemptForAppendLocked(key)
	if err != nil {
		return err
	}
	return p.appendLocked(NewJournalProjectionRecord(attempt.Key, attempt.Lineage, attempt.Version, projection))
}

// AppendRecoveryBlocked records a typed authority or body recovery stop.
func (p *JournalPipeline) AppendRecoveryBlocked(key *SOMutationKey, reason SOJournalRecoveryReason) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, err := p.attemptForAppendLocked(key)
	if err != nil {
		return err
	}
	return p.appendLocked(NewJournalRecoveryBlockedRecord(attempt.Key, attempt.Lineage, attempt.Version, reason))
}

// AppendLineageRecoveryBlocked parks a stale attempt whose body successor is unavailable.
func (p *JournalPipeline) AppendLineageRecoveryBlocked(key *SOMutationKey, reason SOJournalRecoveryReason) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, err := p.attemptForAppendLocked(key)
	if err != nil {
		return err
	}
	return p.appendLocked(NewJournalLineageRecoveryBlockedRecord(attempt.Key, attempt.Lineage, attempt.Version, reason))
}

// Replay returns the durable journal prefix.
func (p *JournalPipeline) Replay() []*SOJournalRecord {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.journal.replay()
}

// AppendIntent allocates the next writer-owned sequence while holding the
// pipeline lock, then durably records an encrypted canonical intent.
func (p *JournalPipeline) AppendIntent(intent *SOJournalIntent, readiness SOJournalReadiness) error {
	if p == nil {
		return ErrJournalStorageRequired
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.crypto == nil {
		return ErrJournalKeyUnavailable
	}
	sequence := p.journal.nextSequence()
	record, err := NewJournalIntentRecord(p.crypto, sequence, intent, readiness, p.journal.writer.identity)
	if err != nil {
		return err
	}
	return p.appendLocked(record)
}

// JournalRecoveryMaterial is the retained canonical intent and immutable envelope.
type JournalRecoveryMaterial struct {
	Intent   *SOJournalIntent
	Envelope []byte
}

// Recover decrypts retained staged material for one exact attempt after reopen.
func (p *JournalPipeline) Recover(key *SOMutationKey) (*JournalRecoveryMaterial, error) {
	if p == nil || p.journal == nil || p.journal.writer == nil || p.crypto == nil {
		return nil, ErrJournalKeyUnavailable
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, err := p.reducer.Attempt(key)
	if err != nil {
		return nil, err
	}
	identity := p.journal.writer.identity
	intentPlaintext, err := p.crypto.OpenWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, attempt.IntentSequence, attempt.Key, identity, attempt.Intent)
	if err != nil {
		return nil, err
	}
	intent := new(SOJournalIntent)
	if err := intent.UnmarshalVT(intentPlaintext); err != nil {
		clear(intentPlaintext)
		return nil, errors.Wrap(ErrJournalCorrupt, "decode retained recovery intent")
	}
	clear(intentPlaintext)
	if !intent.GetKey().EqualExact(attempt.Key) || !intent.GetVersion().EqualVT(attempt.Version) {
		return nil, errors.Wrap(ErrJournalCorrupt, "retained recovery intent identity mismatch")
	}
	material := &JournalRecoveryMaterial{Intent: intent}
	if attempt.Envelope != nil {
		envelope, err := p.crypto.OpenWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE, attempt.EnvelopeSequence, attempt.Key, identity, attempt.Envelope)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(envelope)
		if !bytes.Equal(digest[:], attempt.EnvelopeDigest) {
			clear(envelope)
			return nil, errors.Wrap(ErrJournalCorrupt, "retained recovery envelope digest mismatch")
		}
		material.Envelope = envelope
	}
	return material, nil
}

// AppendEnvelope allocates the next writer-owned sequence while holding the
// pipeline lock, then durably records an encrypted immutable envelope.
func (p *JournalPipeline) AppendEnvelope(intent *SOJournalIntent, envelope []byte) error {
	if p == nil {
		return ErrJournalStorageRequired
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.crypto == nil {
		return ErrJournalKeyUnavailable
	}
	sequence := p.journal.nextSequence()
	record, err := NewJournalEnvelopeRecord(p.crypto, sequence, intent, envelope, p.journal.writer.identity)
	if err != nil {
		return err
	}
	return p.appendLocked(record)
}

// Snapshot returns deterministic reducer state with receipt and projection separate.
func (p *JournalPipeline) Snapshot() []*JournalAttemptSnapshot {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reducer.Snapshot()
}

// Checkpoint publishes the retained journal generation only after the exact
// receipt and body projection are both durable.
func (p *JournalPipeline) Checkpoint(key *SOMutationKey) (*JournalAttemptSnapshot, error) {
	if p == nil {
		return nil, ErrJournalStorageRequired
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	snapshot, err := p.reducer.Checkpoint(key)
	if err != nil {
		return nil, err
	}
	if err := p.journal.checkpoint(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// BeforeSend enforces Q3a's pre-send epoch check and records the send boundary.
// The callback runs after releasing the pipeline lock so transport code can
// safely re-enter observation APIs.
func (p *JournalPipeline) BeforeSend(key *SOMutationKey, currentTransformEpoch uint64, send func() error) error {
	if p == nil {
		return ErrJournalStorageRequired
	}
	if send == nil {
		return errors.Wrap(ErrJournalInvalidTransition, "send callback is required")
	}
	p.mu.Lock()
	attempt, err := p.reducer.Attempt(key)
	if err != nil {
		p.mu.Unlock()
		return err
	}
	if attempt.State == SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT {
		if attempt.Receipt == nil && !attempt.ResendAuthorized {
			p.mu.Unlock()
			return ErrJournalLookupRequired
		}
		if attempt.Receipt != nil {
			p.mu.Unlock()
			return errors.Wrap(ErrJournalInvalidTransition, "terminal receipt already exists")
		}
	} else if attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_ENVELOPE_DURABLE {
		p.mu.Unlock()
		return errors.Wrap(ErrJournalInvalidTransition, "send requires a durable envelope")
	}
	if attempt.Version.GetTransformEpoch() != currentTransformEpoch {
		stale := NewJournalStaleEpochRecord(attempt.Key, attempt.Lineage, attempt.Version)
		if err := p.appendLocked(stale); err != nil {
			p.mu.Unlock()
			return err
		}
		p.mu.Unlock()
		return ErrJournalStaleTransformEpoch
	}
	if attempt.State == SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_ENVELOPE_DURABLE || attempt.ResendAuthorized {
		if err := p.appendLocked(newJournalSentRecord(attempt.Key, attempt.Lineage, attempt.Version)); err != nil {
			p.mu.Unlock()
			return err
		}
	}
	p.mu.Unlock()
	return send()
}

// NewSuccessor constructs a fresh-key body-authorized successor lineage.
func (p *JournalPipeline) NewSuccessor(key *SOMutationKey, successorKey *SOMutationKey, version *SOJournalVersionTuple, operation []byte) (*SOJournalIntent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, err := p.reducer.Attempt(key)
	if err != nil {
		return nil, err
	}
	if attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH && attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED {
		return nil, errors.Wrap(ErrJournalInvalidTransition, "successor requires a recovery-terminal predecessor")
	}
	if err := key.Validate(); err != nil {
		return nil, err
	}
	if err := successorKey.Validate(); err != nil {
		return nil, err
	}
	if !bytes.Equal(successorKey.GetOriginScopeId(), key.GetOriginScopeId()) ||
		successorKey.GetSharedObjectId() != key.GetSharedObjectId() ||
		successorKey.GetParticipantPeerId() != key.GetParticipantPeerId() ||
		successorKey.GetLocalId() == key.GetLocalId() {
		return nil, ErrJournalSupersessionImmutable
	}
	lineage := &SOJournalLineage{RootKey: successorKey.CloneVT(), Supersedes: key.CloneVT()}
	return NewJournalIntent(successorKey, lineage, version, operation)
}

// newJournalSentRecord records the durable send boundary for the exact envelope.
func newJournalSentRecord(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple) *SOJournalRecord {
	return newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SENT, SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT, key, lineage, version, func(*SOJournalRecord) {})
}

// NewJournalReceiptRecord records verified terminal remote evidence.
func NewJournalReceiptRecord(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, receipt *SOJournalReceipt) *SOJournalRecord {
	return newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT, SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE, key, lineage, version, func(record *SOJournalRecord) {
		record.Receipt = receipt.CloneVT()
	})
}

// NewJournalAcknowledgementRecord records acknowledgement without changing receipt evidence.
func NewJournalAcknowledgementRecord(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, acknowledgement *SOJournalAcknowledgement) *SOJournalRecord {
	return newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT, SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE, key, lineage, version, func(record *SOJournalRecord) {
		record.Acknowledgement = acknowledgement.CloneVT()
	})
}

// NewJournalProjectionRecord records the exact authoritative root projected by the body.
func NewJournalProjectionRecord(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, projection *SOJournalProjection) *SOJournalRecord {
	return newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_BODY_PROJECTION, SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE, key, lineage, version, func(record *SOJournalRecord) {
		record.Projection = projection.CloneVT()
	})
}

// NewJournalStaleEpochRecord terminates one exact attempt after verified epoch mismatch.
func NewJournalStaleEpochRecord(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple) *SOJournalRecord {
	return newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_STALE_TRANSFORM_EPOCH, SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH, key, lineage, version, func(record *SOJournalRecord) {
		record.RecoveryReason = SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_STALE_TRANSFORM_EPOCH
	})
}

// NewJournalRecoveryBlockedRecord records a typed authority or body recovery stop.
func NewJournalRecoveryBlockedRecord(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, reason SOJournalRecoveryReason) *SOJournalRecord {
	return newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECOVERY_BLOCKED, SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED, key, lineage, version, func(record *SOJournalRecord) {
		record.RecoveryReason = reason
	})
}

// NewJournalLineageRecoveryBlockedRecord preserves a stale attempt while parking
// its logical lineage when no body-authorized successor can be created.
func NewJournalLineageRecoveryBlockedRecord(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, reasons ...SOJournalRecoveryReason) *SOJournalRecord {
	reason := SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_KEY_UNAVAILABLE
	if len(reasons) > 0 {
		reason = reasons[0]
	}
	return newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_LINEAGE_RECOVERY_BLOCKED, SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH, key, lineage, version, func(record *SOJournalRecord) {
		record.RecoveryReason = reason
	})
}

// NewJournalReceiptLookupRecord records an exact-key receipt observation before
// any recovered resend.
func NewJournalReceiptLookupRecord(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, lookup *SOJournalLookup) *SOJournalRecord {
	state := SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT
	if lookup != nil && (lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_ACCEPTED || lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_REJECTED) {
		state = SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE
	}
	return newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP, state, key, lineage, version, func(record *SOJournalRecord) {
		record.Lookup = lookup.CloneVT()
	})
}

// NewJournalResendAuthorizedRecord records an authoritative no-record lookup.
func NewJournalResendAuthorizedRecord(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple) *SOJournalRecord {
	return newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RESEND_AUTHORIZED, SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT, key, lineage, version, func(*SOJournalRecord) {})
}

// JournalVersion returns a copy of a version tuple for a new attempt.
func JournalVersion(localVersion, remoteVersion, transformEpoch uint64, configChainDigest []byte) *SOJournalVersionTuple {
	return &SOJournalVersionTuple{
		LocalVersion:      localVersion,
		RemoteVersion:     remoteVersion,
		TransformEpoch:    transformEpoch,
		ConfigChainDigest: slices.Clone(configChainDigest),
	}
}
