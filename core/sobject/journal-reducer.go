package sobject

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sync"

	"github.com/pkg/errors"
)

// JournalAttemptSnapshot is the reducer's durable evidence for one exact key.
type JournalAttemptSnapshot struct {
	Key                    *SOMutationKey
	Lineage                *SOJournalLineage
	Version                *SOJournalVersionTuple
	State                  SOJournalAttemptState
	Readiness              SOJournalReadiness
	IntentSequence         uint64
	EnvelopeSequence       uint64
	Intent                 *SOJournalEncryptedPayload
	Envelope               *SOJournalEncryptedPayload
	EnvelopeDigest         []byte
	Receipt                *SOJournalReceipt
	Acknowledgement        *SOJournalAcknowledgement
	Projection             *SOJournalProjection
	Lookup                 *SOJournalLookup
	LookupHistory          []*SOJournalLookup
	SendAttempted          bool
	ResendAuthorized       bool
	LineageRecoveryBlocked bool
	CheckpointEligible     bool
}

// JournalReducer owns live and replay state for body-neutral mutation attempts.
type JournalReducer struct {
	mu       sync.Mutex
	attempts map[string]*JournalAttemptSnapshot
}

// NewJournalReducer constructs an empty deterministic reducer.
func NewJournalReducer() *JournalReducer {
	return &JournalReducer{attempts: make(map[string]*JournalAttemptSnapshot)}
}

// Clone returns an independent reducer candidate for transactional append validation.
func (r *JournalReducer) Clone() *JournalReducer {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := NewJournalReducer()
	for key, attempt := range r.attempts {
		clone.attempts[key] = cloneJournalAttempt(attempt)
	}
	return clone
}

func cloneJournalAttempt(attempt *JournalAttemptSnapshot) *JournalAttemptSnapshot {
	if attempt == nil {
		return nil
	}
	return &JournalAttemptSnapshot{
		Key:                    attempt.Key.CloneVT(),
		Lineage:                attempt.Lineage.CloneVT(),
		Version:                attempt.Version.CloneVT(),
		State:                  attempt.State,
		Readiness:              attempt.Readiness,
		IntentSequence:         attempt.IntentSequence,
		EnvelopeSequence:       attempt.EnvelopeSequence,
		Intent:                 attempt.Intent.CloneVT(),
		Envelope:               attempt.Envelope.CloneVT(),
		EnvelopeDigest:         slices.Clone(attempt.EnvelopeDigest),
		Receipt:                attempt.Receipt.CloneVT(),
		Acknowledgement:        attempt.Acknowledgement.CloneVT(),
		Projection:             attempt.Projection.CloneVT(),
		Lookup:                 attempt.Lookup.CloneVT(),
		LookupHistory:          cloneJournalLookups(attempt.LookupHistory),
		SendAttempted:          attempt.SendAttempted,
		ResendAuthorized:       attempt.ResendAuthorized,
		LineageRecoveryBlocked: attempt.LineageRecoveryBlocked,
		CheckpointEligible:     attempt.CheckpointEligible,
	}
}

func cloneJournalLookups(lookups []*SOJournalLookup) []*SOJournalLookup {
	clones := make([]*SOJournalLookup, len(lookups))
	for index, lookup := range lookups {
		clones[index] = lookup.CloneVT()
	}
	return clones
}

func (r *JournalReducer) hydrate(snapshots []*JournalAttemptSnapshot) error {
	if r == nil {
		return ErrJournalStorageRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, attempt := range snapshots {
		if err := validateCheckpointAttempt(attempt); err != nil {
			return err
		}
		digest, err := MutationKeyDigest(attempt.Key)
		if err != nil {
			return err
		}
		mapKey := hex.EncodeToString(digest)
		if _, exists := r.attempts[mapKey]; exists {
			return errors.Wrap(ErrJournalCheckpointCorrupt, "duplicate checkpoint attempt")
		}
		r.attempts[mapKey] = cloneJournalAttempt(attempt)
	}
	return nil
}

// ReduceJournal replays a validated record sequence into one deterministic reducer.
func ReduceJournal(records []*SOJournalRecord) (*JournalReducer, error) {
	reducer := NewJournalReducer()
	expectedSequence := uint64(1)
	for _, record := range records {
		if record == nil || record.GetSequence() != expectedSequence {
			return nil, errors.Wrap(ErrJournalCorrupt, "journal sequence is not contiguous")
		}
		if err := reducer.Apply(record); err != nil {
			return nil, err
		}
		expectedSequence++
	}
	return reducer, nil
}

// Apply applies one journal transition and commits it to reducer state.
func (r *JournalReducer) Apply(record *SOJournalRecord) error {
	return r.apply(record, true)
}

// validate checks one transition without mutating reducer state.
func (r *JournalReducer) validate(record *SOJournalRecord) error {
	return r.apply(record, false)
}

// Apply applies one journal transition and rejects every illegal transition.
func (r *JournalReducer) apply(record *SOJournalRecord, commit bool) error {
	if err := validateJournalRecord(record); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	keyDigest, err := MutationKeyDigest(record.GetKey())
	if err != nil {
		return err
	}
	mapKey := hex.EncodeToString(keyDigest)
	attempt := r.attempts[mapKey]
	if attempt != nil {
		attempt = cloneJournalAttempt(attempt)
	}
	if attempt == nil {
		if record.GetKind() != SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT {
			return errors.Wrap(ErrJournalInvalidTransition, "attempt must begin with durable intent")
		}
		attempt = &JournalAttemptSnapshot{
			Key:            record.GetKey().CloneVT(),
			Lineage:        cloneJournalLineage(record.GetLineage()),
			Version:        record.GetVersion().CloneVT(),
			State:          record.GetAttemptState(),
			Readiness:      record.GetReadiness(),
			Intent:         record.GetIntent().CloneVT(),
			IntentSequence: record.GetSequence(),
		}
		if supersedes := record.GetLineage().GetSupersedes(); supersedes != nil {
			predecessorDigest, digestErr := MutationKeyDigest(supersedes)
			if digestErr != nil {
				return digestErr
			}
			predecessor := r.attempts[hex.EncodeToString(predecessorDigest)]
			if predecessor == nil || (predecessor.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH && predecessor.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED) {
				return errors.Wrap(ErrJournalInvalidTransition, "successor predecessor is not recovery-terminal")
			}
		}
		if commit {
			r.attempts[mapKey] = attempt
		}
		return nil
	}
	if !attempt.Key.EqualExact(record.GetKey()) || !attempt.Lineage.GetRootKey().EqualExact(record.GetLineage().GetRootKey()) || !sameOptionalKey(attempt.Lineage.GetSupersedes(), record.GetLineage().GetSupersedes()) || !attempt.Version.EqualVT(record.GetVersion()) {
		return ErrJournalSupersessionImmutable
	}
	switch record.GetKind() {
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE:
		if attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_INTENT_DURABLE || attempt.Readiness != SOJournalReadiness_SO_JOURNAL_READINESS_READY || attempt.Envelope != nil || len(record.GetEnvelopeDigest()) != sha256.Size {
			return errors.Wrap(ErrJournalInvalidTransition, "signed envelope requires ready intent")
		}
		attempt.Envelope = record.GetEnvelope().CloneVT()
		attempt.EnvelopeSequence = record.GetSequence()
		attempt.EnvelopeDigest = slices.Clone(record.GetEnvelopeDigest())
		attempt.State = record.GetAttemptState()
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SENT:
		if attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_ENVELOPE_DURABLE && (attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT || !attempt.ResendAuthorized) {
			return errors.Wrap(ErrJournalInvalidTransition, "send completion requires an unsent durable envelope")
		}
		if attempt.ResendAuthorized {
			attempt.Lookup = nil
		}
		attempt.SendAttempted = true
		attempt.ResendAuthorized = false
		attempt.State = record.GetAttemptState()
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT:
		if attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT || !attempt.SendAttempted || attempt.Receipt != nil {
			return errors.Wrap(ErrJournalInvalidTransition, "receipt requires one attempted send")
		}
		if !validJournalReceipt(record.GetReceipt(), record.GetKey(), record.GetLineage(), attempt.Version, attempt.EnvelopeDigest) {
			return errors.Wrap(ErrJournalInvalidTransition, "receipt is not bound to the durable envelope")
		}
		attempt.Receipt = record.GetReceipt().CloneVT()
		attempt.ResendAuthorized = false
		attempt.State = record.GetAttemptState()
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP:
		if attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT || !attempt.SendAttempted || attempt.Receipt != nil {
			return errors.Wrap(ErrJournalInvalidTransition, "receipt lookup requires one ambiguous send")
		}
		if attempt.Lookup != nil && attempt.Lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_PENDING {
			return errors.Wrap(ErrJournalInvalidTransition, "receipt lookup already resolved for this send")
		}
		lookup := record.GetLookup()
		if !lookup.GetKey().EqualExact(record.GetKey()) {
			return errors.Wrap(ErrJournalInvalidKey, "lookup key differs from attempt key")
		}
		if attempt.Lookup != nil && attempt.Lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_PENDING && lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_NO_RECORD {
			return errors.Wrap(ErrJournalInvalidTransition, "pending receipt lookup cannot regress to no-record")
		}
		attempt.Lookup = lookup.CloneVT()
		attempt.LookupHistory = []*SOJournalLookup{lookup.CloneVT()}
		if lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_ACCEPTED || lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_REJECTED {
			if !validJournalReceipt(lookup.GetReceipt(), record.GetKey(), record.GetLineage(), attempt.Version, attempt.EnvelopeDigest) {
				return errors.Wrap(ErrJournalInvalidTransition, "lookup terminal receipt is not bound")
			}
			attempt.Receipt = lookup.GetReceipt().CloneVT()
			attempt.ResendAuthorized = false
			attempt.State = SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RESEND_AUTHORIZED:
		if attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT || attempt.Lookup == nil || attempt.Lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_NO_RECORD || attempt.ResendAuthorized {
			return errors.Wrap(ErrJournalInvalidTransition, "resend requires an authoritative no-record lookup")
		}
		attempt.ResendAuthorized = true
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT:
		if attempt.Receipt == nil || !bytes.Equal(attempt.Receipt.GetTerminalReceiptDigest(), record.GetAcknowledgement().GetReceiptDigest()) {
			return errors.Wrap(ErrJournalInvalidTransition, "acknowledgement does not match durable receipt")
		}
		if attempt.Acknowledgement != nil && !attempt.Acknowledgement.EqualVT(record.GetAcknowledgement()) {
			return errors.Wrap(ErrJournalInvalidTransition, "acknowledgement is immutable")
		}
		attempt.Acknowledgement = record.GetAcknowledgement().CloneVT()
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_BODY_PROJECTION:
		if attempt.Receipt == nil || !bytes.Equal(attempt.Receipt.GetTerminalReceiptDigest(), record.GetProjection().GetReceiptDigest()) || attempt.Receipt.GetAuthoritativeRootSeqno() != record.GetProjection().GetAuthoritativeRootSeqno() || !bytes.Equal(attempt.Receipt.GetAuthoritativeRootDigest(), record.GetProjection().GetAuthoritativeRootDigest()) {
			return errors.Wrap(ErrJournalInvalidTransition, "projection does not match exact receipt root")
		}
		if attempt.Projection != nil && !attempt.Projection.EqualVT(record.GetProjection()) {
			return errors.Wrap(ErrJournalInvalidTransition, "body projection is immutable")
		}
		attempt.Projection = record.GetProjection().CloneVT()
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_STALE_TRANSFORM_EPOCH:
		if attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_INTENT_DURABLE && attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_ENVELOPE_DURABLE && attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT {
			return errors.Wrap(ErrJournalInvalidTransition, "stale epoch must terminate an unsatisfied attempt")
		}
		if attempt.State == SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT && !authoritativeLookupComplete(attempt.Lookup) {
			return errors.Wrap(ErrJournalInvalidTransition, "stale epoch requires an authoritative receipt lookup")
		}
		attempt.ResendAuthorized = false
		attempt.State = record.GetAttemptState()
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECOVERY_BLOCKED:
		if attempt.State == SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE || attempt.State == SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH || attempt.State == SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED {
			return errors.Wrap(ErrJournalInvalidTransition, "recovery stop cannot replace terminal attempt")
		}
		if attempt.State == SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT && !authoritativeLookupComplete(attempt.Lookup) {
			return errors.Wrap(ErrJournalInvalidTransition, "recovery stop requires an authoritative receipt lookup")
		}
		if !readinessMatchesRecovery(attempt.Readiness, record.GetRecoveryReason()) {
			return errors.Wrap(ErrJournalInvalidTransition, "recovery reason does not match body readiness")
		}
		attempt.ResendAuthorized = false
		attempt.State = record.GetAttemptState()
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_LINEAGE_RECOVERY_BLOCKED:
		if attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH || attempt.LineageRecoveryBlocked {
			return errors.Wrap(ErrJournalInvalidTransition, "lineage recovery block requires stale attempt")
		}
		attempt.LineageRecoveryBlocked = true
	default:
		return ErrJournalInvalidTransition
	}
	attempt.CheckpointEligible = attempt.Receipt != nil && attempt.Projection != nil
	if commit {
		r.attempts[mapKey] = attempt
	}
	return nil
}

// Snapshot returns all attempts sorted by their exact mutation-key digest.
func (r *JournalReducer) Snapshot() []*JournalAttemptSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*JournalAttemptSnapshot, 0, len(r.attempts))
	for _, attempt := range r.attempts {
		clone := cloneJournalAttempt(attempt)
		out = append(out, clone)
	}
	slices.SortFunc(out, func(a, b *JournalAttemptSnapshot) int {
		da, _ := MutationKeyDigest(a.Key)
		db, _ := MutationKeyDigest(b.Key)
		return bytes.Compare(da, db)
	})
	return out
}

// Attempt returns a copy of one exact participant-scoped attempt.
func (r *JournalReducer) Attempt(key *SOMutationKey) (*JournalAttemptSnapshot, error) {
	digest, err := MutationKeyDigest(key)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt := r.attempts[hex.EncodeToString(digest)]
	if attempt == nil {
		return nil, errors.Wrap(ErrJournalInvalidKey, "unknown journal attempt")
	}
	return (&JournalReducer{attempts: map[string]*JournalAttemptSnapshot{hex.EncodeToString(digest): attempt}}).Snapshot()[0], nil
}

// Checkpoint returns retained evidence only after the exact receipt root is projected.
func (r *JournalReducer) Checkpoint(key *SOMutationKey) (*JournalAttemptSnapshot, error) {
	attempt, err := r.Attempt(key)
	if err != nil {
		return nil, err
	}
	if !attempt.CheckpointEligible {
		return nil, errors.Wrap(ErrJournalInvalidTransition, "exact authoritative projection is required before checkpoint")
	}
	return attempt, nil
}

func validateJournalRecord(record *SOJournalRecord) error {
	if record == nil || record.GetFormatVersion() != JournalFormatVersion || record.GetSequence() == 0 || !validJournalRecordKind(record.GetKind()) {
		return ErrJournalCorrupt
	}
	if err := validateJournalLineage(record.GetKey(), record.GetLineage()); err != nil {
		return err
	}
	if record.GetVersion() == nil {
		return errors.Wrap(ErrJournalCorrupt, "journal version tuple is required")
	}
	switch record.GetKind() {
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT:
		if record.GetAttemptState() != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_INTENT_DURABLE || !validJournalReadiness(record.GetReadiness()) || !validEncryptedPayload(record.GetIntent()) {
			return ErrJournalInvalidTransition
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE:
		if record.GetAttemptState() != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_ENVELOPE_DURABLE || !validEncryptedPayload(record.GetEnvelope()) || len(record.GetEnvelopeDigest()) != sha256.Size {
			return ErrJournalInvalidTransition
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SENT:
		if record.GetAttemptState() != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT {
			return ErrJournalInvalidTransition
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT:
		if record.GetAttemptState() != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE || !validJournalReceipt(record.GetReceipt(), record.GetKey(), record.GetLineage(), nil, nil) {
			return ErrJournalInvalidTransition
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP:
		lookup := record.GetLookup()
		expectedState := SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT
		if lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_ACCEPTED || lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_REJECTED {
			expectedState = SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE
		}
		if record.GetAttemptState() != expectedState || !validJournalLookup(lookup, record.GetKey(), record.GetLineage(), record.GetVersion()) {
			return ErrJournalInvalidTransition
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RESEND_AUTHORIZED:
		if record.GetAttemptState() != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT {
			return ErrJournalInvalidTransition
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT:
		if record.GetAttemptState() != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE || !validJournalAcknowledgement(record.GetAcknowledgement(), record.GetKey()) {
			return ErrJournalInvalidTransition
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_BODY_PROJECTION:
		if record.GetAttemptState() != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE || !validJournalProjection(record.GetProjection(), record.GetKey()) {
			return ErrJournalInvalidTransition
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_STALE_TRANSFORM_EPOCH:
		if record.GetAttemptState() != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH || record.GetRecoveryReason() != SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_STALE_TRANSFORM_EPOCH {
			return ErrJournalInvalidTransition
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECOVERY_BLOCKED:
		if record.GetAttemptState() != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED || !validJournalRecoveryReason(record.GetRecoveryReason()) {
			return ErrJournalInvalidTransition
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_LINEAGE_RECOVERY_BLOCKED:
		if record.GetAttemptState() != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH || !validJournalRecoveryReason(record.GetRecoveryReason()) || record.GetRecoveryReason() == SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_STALE_TRANSFORM_EPOCH {
			return ErrJournalInvalidTransition
		}
	}
	return nil
}

func validJournalAttemptState(state SOJournalAttemptState) bool {
	return state >= SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_INTENT_DURABLE &&
		state <= SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED
}

func validEncryptedPayload(payload *SOJournalEncryptedPayload) bool {
	return payload != nil && len(payload.GetNonce()) == journalNonceSize && len(payload.GetCiphertext()) >= 16
}

func validJournalReadiness(readiness SOJournalReadiness) bool {
	return readiness == SOJournalReadiness_SO_JOURNAL_READINESS_READY ||
		readiness == SOJournalReadiness_SO_JOURNAL_READINESS_MISSING ||
		readiness == SOJournalReadiness_SO_JOURNAL_READINESS_CORRUPT ||
		readiness == SOJournalReadiness_SO_JOURNAL_READINESS_OBSOLETE
}

func validJournalOutcome(outcome SOJournalOutcome) bool {
	return outcome == SOJournalOutcome_SO_JOURNAL_OUTCOME_ACCEPTED || outcome == SOJournalOutcome_SO_JOURNAL_OUTCOME_REJECTED
}

func validJournalRecoveryReason(reason SOJournalRecoveryReason) bool {
	return reason >= SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_STALE_TRANSFORM_EPOCH &&
		reason <= SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_BODY_OBSOLETE
}

func validJournalLookupState(state SOReceiptState) bool {
	return state >= SOReceiptState_SO_RECEIPT_STATE_NO_RECORD &&
		state <= SOReceiptState_SO_RECEIPT_STATE_REJECTED
}

func readinessMatchesRecovery(readiness SOJournalReadiness, reason SOJournalRecoveryReason) bool {
	switch readiness {
	case SOJournalReadiness_SO_JOURNAL_READINESS_MISSING:
		return reason == SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_BODY_MISSING
	case SOJournalReadiness_SO_JOURNAL_READINESS_CORRUPT:
		return reason == SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_BODY_CORRUPT
	case SOJournalReadiness_SO_JOURNAL_READINESS_OBSOLETE:
		return reason == SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_BODY_OBSOLETE
	default:
		return reason == SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_KEY_UNAVAILABLE ||
			reason == SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_AUTHORITY_FAILURE
	}
}

func validJournalReceipt(receipt *SOJournalReceipt, key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, envelopeDigest []byte) bool {
	if receipt == nil || !receipt.GetKey().EqualExact(key) || !sameOptionalKey(receipt.GetSupersedes(), lineage.GetSupersedes()) ||
		!validJournalOutcome(receipt.GetOutcome()) || len(receipt.GetEnvelopeDigest()) != sha256.Size ||
		len(receipt.GetTerminalReceipt()) == 0 || len(receipt.GetTerminalReceiptDigest()) != sha256.Size ||
		len(receipt.GetAuthoritativeRootDigest()) != sha256.Size || len(receipt.GetConfigChainDigest()) != sha256.Size {
		return false
	}
	terminalDigest := sha256.Sum256(receipt.GetTerminalReceipt())
	if !bytes.Equal(terminalDigest[:], receipt.GetTerminalReceiptDigest()) {
		return false
	}
	if len(envelopeDigest) > 0 && !bytes.Equal(envelopeDigest, receipt.GetEnvelopeDigest()) {
		return false
	}
	if version != nil && !bytes.Equal(version.GetConfigChainDigest(), receipt.GetConfigChainDigest()) {
		return false
	}
	return true
}

func validJournalLookup(lookup *SOJournalLookup, key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple) bool {
	if lookup == nil || !lookup.GetKey().EqualExact(key) || !validJournalLookupState(lookup.GetState()) ||
		len(lookup.GetResponse()) == 0 || len(lookup.GetResponseDigest()) != sha256.Size ||
		len(lookup.GetConfigChainDigest()) != sha256.Size {
		return false
	}
	responseDigest := sha256.Sum256(lookup.GetResponse())
	if !bytes.Equal(responseDigest[:], lookup.GetResponseDigest()) {
		return false
	}
	if version != nil && !bytes.Equal(version.GetConfigChainDigest(), lookup.GetConfigChainDigest()) {
		return false
	}
	if lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_ACCEPTED || lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_REJECTED {
		receipt := lookup.GetReceipt()
		if !validJournalReceipt(receipt, key, lineage, version, nil) {
			return false
		}
		if lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_ACCEPTED {
			return receipt.GetOutcome() == SOJournalOutcome_SO_JOURNAL_OUTCOME_ACCEPTED
		}
		return receipt.GetOutcome() == SOJournalOutcome_SO_JOURNAL_OUTCOME_REJECTED
	}
	return lookup.GetReceipt() == nil
}

func authoritativeLookupComplete(lookup *SOJournalLookup) bool {
	return lookup != nil && lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_UNSPECIFIED &&
		lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_PENDING
}

func validJournalAcknowledgement(ack *SOJournalAcknowledgement, key *SOMutationKey) bool {
	return ack != nil && ack.GetKey().EqualExact(key) && len(ack.GetReceiptDigest()) == sha256.Size
}

func validJournalProjection(projection *SOJournalProjection, key *SOMutationKey) bool {
	return projection != nil && projection.GetKey().EqualExact(key) && len(projection.GetReceiptDigest()) == sha256.Size && len(projection.GetAuthoritativeRootDigest()) == sha256.Size
}

func sameOptionalKey(a, b *SOMutationKey) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.EqualExact(b)
}

// NewJournalIntent constructs the canonical body-neutral intent payload.
func NewJournalIntent(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, operation []byte) (*SOJournalIntent, error) {
	if err := validateJournalLineage(key, lineage); err != nil {
		return nil, err
	}
	if version == nil || len(operation) == 0 {
		return nil, errors.Wrap(ErrJournalCorrupt, "journal intent is incomplete")
	}
	return &SOJournalIntent{
		Key:                key.CloneVT(),
		Lineage:            lineage.CloneVT(),
		Version:            version.CloneVT(),
		CanonicalOperation: slices.Clone(operation),
	}, nil
}

// NewJournalIntentRecord encrypts and wraps a canonical intent for durable append.
func NewJournalIntentRecord(crypto *JournalCrypto, sequence uint64, intent *SOJournalIntent, readiness SOJournalReadiness, identity []byte) (*SOJournalRecord, error) {
	if intent == nil || sequence == 0 {
		return nil, errors.Wrap(ErrJournalCorrupt, "journal intent is incomplete")
	}
	if !validJournalReadiness(readiness) {
		return nil, errors.Wrap(ErrJournalRecoveryBlocked, "body readiness is required")
	}
	payload, err := intent.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal journal intent")
	}
	defer func() { clear(payload) }()
	encrypted, err := crypto.SealWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, sequence, intent.GetKey(), identity, payload)
	if err != nil {
		return nil, err
	}
	record := newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_INTENT_DURABLE, intent.GetKey(), intent.GetLineage(), intent.GetVersion(), func(record *SOJournalRecord) {
		record.Intent = encrypted
		record.Readiness = readiness
	})
	record.Sequence = sequence
	return record, nil
}

// NewJournalEnvelopeRecord encrypts immutable signed-envelope bytes for durable append.
func NewJournalEnvelopeRecord(crypto *JournalCrypto, sequence uint64, intent *SOJournalIntent, envelope, identity []byte) (*SOJournalRecord, error) {
	if intent == nil || sequence == 0 || len(envelope) == 0 {
		return nil, errors.Wrap(ErrJournalCorrupt, "journal envelope is incomplete")
	}
	encrypted, err := crypto.SealWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE, sequence, intent.GetKey(), identity, envelope)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(envelope)
	record := newJournalRecord(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE, SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_ENVELOPE_DURABLE, intent.GetKey(), intent.GetLineage(), intent.GetVersion(), func(record *SOJournalRecord) {
		record.Envelope = encrypted
		record.EnvelopeDigest = slices.Clone(digest[:])
	})
	record.Sequence = sequence
	return record, nil
}

func newJournalRecord(kind SOJournalRecordKind, state SOJournalAttemptState, key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, fill func(*SOJournalRecord)) *SOJournalRecord {
	record := &SOJournalRecord{
		FormatVersion: JournalFormatVersion,
		Kind:          kind,
		Key:           key.CloneVT(),
		Lineage:       lineage.CloneVT(),
		Version:       version.CloneVT(),
		AttemptState:  state,
	}
	fill(record)
	return record
}
