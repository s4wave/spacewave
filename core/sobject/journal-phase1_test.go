package sobject

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPhase1MutationKeyAndTransitionCorpus(t *testing.T) {
	// Build mutation keys, lineages, and version fixtures.
	scope := testScope("scope")
	keyA := testMutationKey(scope, "peer-a", "same-local")
	keyB := testMutationKey(scope, "peer-b", "same-local")
	lineageA := testLineage(keyA, nil)
	version := JournalVersion(1, 2, 3, testDigest("config"))

	if keyA.EqualExact(keyB) {
		t.Fatal("participant identity must separate same local_id")
	}
	if _, err := NewSOMutationKey([]byte("short"), "so", "peer", "local"); !errors.Is(err, ErrJournalInvalidKey) {
		t.Fatalf("short origin scope accepted: %v", err)
	}

	// Append valid intents and verify participant separation.
	crypto := testJournalCrypto(t, scope)
	intentA := testIntent(t, crypto, keyA, lineageA, version, 1, "world-neutral-op")
	intentB := testIntent(t, crypto, keyB, testLineage(keyB, nil), version, 2, "non-world-op")
	pipeline := testPipeline(t, crypto)
	if err := pipeline.appendRecord(intentA); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(intentB); err != nil {
		t.Fatal(err)
	}
	if got := len(pipeline.Snapshot()); got != 2 {
		t.Fatalf("participant-separated attempts = %d, want 2", got)
	}

	// Reject an envelope with a changed version tuple.
	badVersion := JournalVersion(4, 2, 3, testDigest("config"))
	badEnvelope, err := NewJournalEnvelopeRecord(crypto, 3, &SOJournalIntent{Key: keyA, Lineage: lineageA, Version: badVersion, CanonicalOperation: []byte("changed")}, []byte("signed-envelope"), journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(badEnvelope); !errors.Is(err, ErrJournalSupersessionImmutable) {
		t.Fatalf("changed tuple accepted: %v", err)
	}

	// Append valid terminal evidence and verify checkpoint readiness.
	validEnvelope, err := NewJournalEnvelopeRecord(crypto, 3, testDecodedIntent(t, crypto, intentA, keyA, lineageA, version, 1), []byte("signed-envelope"), journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(validEnvelope); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(newJournalSentRecord(keyA, lineageA, version)); err != nil {
		t.Fatal(err)
	}

	receipt := testReceipt(keyA, testDigest("signed-envelope"), []byte("receipt"), 9, testDigest("root"))
	if err := pipeline.appendRecord(NewJournalReceiptRecord(keyA, lineageA, version, receipt)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := pipeline.reducer.Attempt(keyA)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Receipt == nil || snapshot.Projection != nil || snapshot.CheckpointEligible {
		t.Fatal("receipt terminality implied body projection or checkpoint eligibility")
	}
	ack := &SOJournalAcknowledgement{Key: keyA.CloneVT(), ReceiptDigest: testDigest("receipt")}
	ack.ReceiptDigest = receipt.GetTerminalReceiptDigest()
	if err := pipeline.appendRecord(NewJournalAcknowledgementRecord(keyA, lineageA, version, ack)); err != nil {
		t.Fatal(err)
	}
	if snapshot, err = pipeline.reducer.Attempt(keyA); err != nil {
		t.Fatal(err)
	} else if snapshot.CheckpointEligible {
		t.Fatal("acknowledgement implied checkpoint eligibility")
	}
	projection := &SOJournalProjection{Key: keyA.CloneVT(), ReceiptDigest: receipt.GetTerminalReceiptDigest(), AuthoritativeRootSeqno: 9, AuthoritativeRootDigest: testDigest("root")}
	if err := pipeline.appendRecord(NewJournalProjectionRecord(keyA, lineageA, version, projection)); err != nil {
		t.Fatal(err)
	}
	if snapshot, err = pipeline.reducer.Attempt(keyA); err != nil {
		t.Fatal(err)
	} else if !snapshot.CheckpointEligible {
		t.Fatal("exact authoritative projection did not enable checkpoint")
	}
	checkpoint, err := pipeline.Checkpoint(keyA)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Intent == nil || checkpoint.Envelope == nil || checkpoint.Receipt == nil || checkpoint.Projection == nil {
		t.Fatal("checkpoint dropped staged or terminal evidence")
	}

	// Reject transitions with missing or unknown terminal state.
	illegal := NewJournalReceiptRecord(keyB, testLineage(keyB, nil), version, testReceipt(keyB, []byte("e"), []byte("r"), 1, []byte("root")))
	illegal.Sequence = uint64(len(pipeline.Replay()) + 1)
	if err := NewJournalReducer().Apply(illegal); !errors.Is(err, ErrJournalInvalidTransition) {
		t.Fatalf("receipt without intent accepted: %v", err)
	}

	unknownReadiness := testIntentRecord(keyB, testLineage(keyB, nil), version)
	unknownReadiness.Readiness = SOJournalReadiness(99)
	if err := NewJournalReducer().Apply(unknownReadiness); !errors.Is(err, ErrJournalInvalidTransition) {
		t.Fatalf("unknown readiness accepted: %v", err)
	}
	unknownReceipt := testReceipt(keyB, testDigest("envelope"), []byte("receipt"), 1, testDigest("root"))
	unknownReceipt.Outcome = SOJournalOutcome(99)
	unknownReceiptRecord := NewJournalReceiptRecord(keyB, testLineage(keyB, nil), version, unknownReceipt)
	unknownReceiptRecord.Sequence = 1
	if err := validateJournalRecord(unknownReceiptRecord); !errors.Is(err, ErrJournalInvalidTransition) {
		t.Fatalf("unknown receipt outcome accepted: %v", err)
	}
}

func TestPhase1TransitionTable(t *testing.T) {
	scope := testScope("transition-table")
	crypto := testJournalCrypto(t, scope)
	key := testMutationKey(scope, "peer", "table")
	lineage := testLineage(key, nil)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	intent := testIntent(t, crypto, key, lineage, version, 1, "opaque")
	decoded := testDecodedIntent(t, crypto, intent, key, lineage, version, 1)
	envelope, err := NewJournalEnvelopeRecord(crypto, 2, decoded, []byte("envelope"), journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	envelope.Sequence = 1
	receipt := testReceipt(key, testDigest("envelope"), []byte("terminal"), 1, testDigest("root"))
	lookup := &SOJournalLookup{
		Key:               key.CloneVT(),
		State:             SOReceiptState_SO_RECEIPT_STATE_NO_RECORD,
		Response:          []byte("no-record"),
		ResponseDigest:    testDigest("no-record"),
		ConfigChainDigest: testDigest("config"),
	}
	records := []*SOJournalRecord{
		intent,
		envelope,
		newJournalSentRecord(key, lineage, version),
		NewJournalReceiptRecord(key, lineage, version, receipt),
		NewJournalReceiptLookupRecord(key, lineage, version, lookup),
		NewJournalResendAuthorizedRecord(key, lineage, version),
		NewJournalAcknowledgementRecord(key, lineage, version, &SOJournalAcknowledgement{Key: key.CloneVT(), ReceiptDigest: testDigest("terminal")}),
		NewJournalProjectionRecord(key, lineage, version, &SOJournalProjection{Key: key.CloneVT(), ReceiptDigest: testDigest("terminal"), AuthoritativeRootSeqno: 1, AuthoritativeRootDigest: testDigest("root")}),
		NewJournalStaleEpochRecord(key, lineage, version),
		NewJournalRecoveryBlockedRecord(key, lineage, version, SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_KEY_UNAVAILABLE),
		NewJournalLineageRecoveryBlockedRecord(key, lineage, version),
	}
	for index, record := range records {
		record.Sequence = 1
		err := NewJournalReducer().Apply(record)
		if index == 0 {
			if err != nil {
				t.Fatalf("intent initial transition rejected: %v", err)
			}
			continue
		}
		if !errors.Is(err, ErrJournalInvalidTransition) {
			t.Fatalf("initial %s transition error = %v", record.GetKind(), err)
		}
	}
}

func TestPhase1ExhaustiveTransitionMatrix(t *testing.T) {
	scope := testScope("transition-matrix")
	crypto := testJournalCrypto(t, scope)
	key := testMutationKey(scope, "peer", "matrix")
	lineage := testLineage(key, nil)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	intent := testIntent(t, crypto, key, lineage, version, 1, "matrix")
	decoded := testDecodedIntent(t, crypto, intent, key, lineage, version, 1)
	envelope, err := NewJournalEnvelopeRecord(crypto, 2, decoded, []byte("matrix-envelope"), journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	receipt := testReceipt(key, testDigest("matrix-envelope"), []byte("matrix-terminal"), 1, testDigest("matrix-root"))
	ack := &SOJournalAcknowledgement{Key: key.CloneVT(), ReceiptDigest: receipt.GetTerminalReceiptDigest()}
	projection := &SOJournalProjection{Key: key.CloneVT(), ReceiptDigest: receipt.GetTerminalReceiptDigest(), AuthoritativeRootSeqno: 1, AuthoritativeRootDigest: testDigest("matrix-root")}
	lookup := &SOJournalLookup{Key: key.CloneVT(), State: SOReceiptState_SO_RECEIPT_STATE_NO_RECORD, Response: []byte("matrix-no-record"), ResponseDigest: testDigest("matrix-no-record"), ConfigChainDigest: testDigest("config")}
	candidates := map[SOJournalRecordKind]*SOJournalRecord{
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT:                   intent,
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE:          envelope,
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SENT:                     newJournalSentRecord(key, lineage, version),
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT:                  NewJournalReceiptRecord(key, lineage, version, receipt),
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP:           NewJournalReceiptLookupRecord(key, lineage, version, lookup),
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RESEND_AUTHORIZED:        NewJournalResendAuthorizedRecord(key, lineage, version),
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT:          NewJournalAcknowledgementRecord(key, lineage, version, ack),
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_BODY_PROJECTION:          NewJournalProjectionRecord(key, lineage, version, projection),
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_STALE_TRANSFORM_EPOCH:    NewJournalStaleEpochRecord(key, lineage, version),
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECOVERY_BLOCKED:         NewJournalRecoveryBlockedRecord(key, lineage, version, SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_KEY_UNAVAILABLE),
		SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_LINEAGE_RECOVERY_BLOCKED: NewJournalLineageRecoveryBlockedRecord(key, lineage, version),
	}
	prefixes := [][]*SOJournalRecord{
		nil,
		{intent},
		{intent, envelope},
		{intent, envelope, candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SENT]},
		{intent, envelope, candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SENT], candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT]},
		{intent, envelope, candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SENT], candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT], candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT]},
		{intent, envelope, candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SENT], candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT], candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT], candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_BODY_PROJECTION]},
	}
	legal := []map[SOJournalRecordKind]bool{
		{SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT: true},
		{SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE: true, SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_STALE_TRANSFORM_EPOCH: true, SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECOVERY_BLOCKED: true},
		{SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SENT: true, SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_STALE_TRANSFORM_EPOCH: true, SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECOVERY_BLOCKED: true},
		{SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT: true, SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP: true},
		{SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT: true, SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_BODY_PROJECTION: true},
		{SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT: true, SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_BODY_PROJECTION: true},
		{SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT: true, SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_BODY_PROJECTION: true},
	}
	for state, prefix := range prefixes {
		for kind, candidate := range candidates {
			reducer := NewJournalReducer()
			for index, record := range prefix {
				copy := record.CloneVT()
				copy.Sequence = uint64(index + 1)
				if err := reducer.Apply(copy); err != nil {
					t.Fatalf("prefix state %d rejected %s: %v", state, record.GetKind(), err)
				}
			}
			candidateCopy := candidate.CloneVT()
			candidateCopy.Sequence = uint64(len(prefix) + 1)
			err := reducer.Apply(candidateCopy)
			if legal[state][kind] {
				if err != nil {
					t.Errorf("state %d rejected legal %s: %v", state, kind, err)
				}
			} else if err == nil {
				t.Errorf("state %d accepted illegal %s", state, kind)
			}
		}
	}
	resend := NewJournalReducer()
	for index, record := range []*SOJournalRecord{intent, envelope, candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SENT], candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP], candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RESEND_AUTHORIZED]} {
		copy := record.CloneVT()
		copy.Sequence = uint64(index + 1)
		if err := resend.Apply(copy); err != nil {
			t.Fatalf("resend path rejected %s: %v", record.GetKind(), err)
		}
	}
	stale := NewJournalReducer()
	for index, record := range []*SOJournalRecord{intent, envelope, candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_STALE_TRANSFORM_EPOCH], candidates[SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_LINEAGE_RECOVERY_BLOCKED]} {
		copy := record.CloneVT()
		copy.Sequence = uint64(index + 1)
		if err := stale.Apply(copy); err != nil {
			t.Fatalf("lineage-block path rejected %s: %v", record.GetKind(), err)
		}
	}
}

func TestPhase1JournalFramingAndStickyFailure(t *testing.T) {
	scope := testScope("framing")
	crypto := testJournalCrypto(t, scope)
	key := testMutationKey(scope, "peer", "local")
	lineage := testLineage(key, nil)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	record := testIntent(t, crypto, key, lineage, version, 1, "framing")
	storage := newMemoryJournalStorage()
	writer, _, err := openJournalWriter(storage, crypto)
	if err != nil {
		t.Fatal(err)
	}

	mismatchedSequence := record.CloneVT()
	mismatchedSequence.Sequence = 9
	if err := writer.Append(mismatchedSequence); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("writer accepted caller-owned sequence: %v", err)
	}
	if err := writer.Append(record); err != nil {
		t.Fatal(err)
	}
	validSize, err := storage.Size()
	if err != nil {
		t.Fatal(err)
	}
	validBytes := storage.bytes()

	if _, _, err := openJournalWriter(nil); !errors.Is(err, ErrJournalStorageRequired) {
		t.Fatalf("nil journal storage error = %v", err)
	}
	if _, err := openJournal(nil); !errors.Is(err, ErrJournalStorageRequired) {
		t.Fatalf("nil journal open error = %v", err)
	}
	if _, err := OpenJournalPipeline(nil); !errors.Is(err, ErrJournalStorageRequired) {
		t.Fatalf("nil journal pipeline error = %v", err)
	}
	partialHeader := append([]byte{}, journalMagic[:]...)
	partialHeader = append(partialHeader, 0x00, byte(journalFrameVersion))
	if _, err := storage.WriteAt(partialHeader, validSize); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(storage, crypto); err != nil {
		t.Fatalf("valid incomplete final header rejected: %v", err)
	}
	if got, _ := storage.Size(); got != validSize {
		t.Fatalf("torn tail was not truncated: got %d want %d", got, validSize)
	}
	if _, _, err := openJournalWriter(storage, crypto); err != nil {
		t.Fatalf("reopen after torn-tail truncation failed: %v", err)
	}
	torn := newMemoryJournalStorage()
	if _, err := torn.WriteAt(validBytes, 0); err != nil {
		t.Fatal(err)
	}
	tailFrame, err := marshalJournalFrame(record.GetKind(), 2, validBytes[journalHeaderSize:len(validBytes)-journalTrailerSize])
	if err != nil {
		t.Fatal(err)
	}
	for prefixLen := 1; prefixLen < journalHeaderSize; prefixLen++ {
		prefixStorage := newMemoryJournalStorage()
		if _, err := prefixStorage.WriteAt(validBytes, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := prefixStorage.WriteAt(tailFrame[:prefixLen], validSize); err != nil {
			t.Fatal(err)
		}
		if _, _, err := openJournalWriter(prefixStorage, crypto); err != nil {
			t.Fatalf("valid incomplete header prefix %d rejected: %v", prefixLen, err)
		}
		if got, _ := prefixStorage.Size(); got != validSize {
			t.Fatalf("header prefix %d was not truncated: got %d want %d", prefixLen, got, validSize)
		}
	}
	for _, prefix := range [][]byte{
		append([]byte{}, journalMagic[0], journalMagic[1], journalMagic[2], journalMagic[3], 0xff),
		append([]byte{}, journalMagic[0], journalMagic[1], journalMagic[2], journalMagic[3], 0, byte(journalFrameVersion), 0xff),
	} {
		impossiblePrefix := newMemoryJournalStorage()
		if _, err := impossiblePrefix.WriteAt(validBytes, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := impossiblePrefix.WriteAt(prefix, validSize); err != nil {
			t.Fatal(err)
		}
		if _, _, err := openJournalWriter(impossiblePrefix, crypto); !errors.Is(err, ErrJournalCorrupt) {
			t.Fatalf("impossible header prefix accepted: %v", err)
		}
	}
	impossibleSequence := newMemoryJournalStorage()
	if _, err := impossibleSequence.WriteAt(validBytes, 0); err != nil {
		t.Fatal(err)
	}
	badSequencePrefix := append([]byte(nil), tailFrame[:16]...)
	badSequencePrefix[15]++
	if _, err := impossibleSequence.WriteAt(badSequencePrefix, validSize); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(impossibleSequence, crypto); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatal("impossible sequence prefix accepted")
	}
	if _, err := torn.WriteAt(tailFrame[:len(tailFrame)-1], validSize); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(torn, crypto); err != nil {
		t.Fatalf("valid incomplete final payload rejected: %v", err)
	}
	if got, _ := torn.Size(); got != validSize {
		t.Fatalf("incomplete payload was not truncated: got %d want %d", got, validSize)
	}
	gap := newMemoryJournalStorage()
	if _, err := gap.WriteAt(validBytes, 0); err != nil {
		t.Fatal(err)
	}
	gapFrame, err := marshalJournalFrame(record.GetKind(), 3, validBytes[journalHeaderSize:len(validBytes)-journalTrailerSize])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gap.WriteAt(gapFrame, validSize); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(gap, crypto); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("non-contiguous journal sequence accepted: %v", err)
	}

	corrupt := newMemoryJournalStorage()
	if _, err := corrupt.WriteAt(validBytes, 0); err != nil {
		t.Fatal(err)
	}
	corruptByte := validBytes[journalHeaderSize]
	if _, err := corrupt.WriteAt([]byte{corruptByte ^ 0xff}, journalHeaderSize); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(corrupt, crypto); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("complete corruption did not fail closed: %v", err)
	}
	if got, _ := corrupt.Size(); got != int64(len(validBytes)) {
		t.Fatal("complete corruption was truncated")
	}

	lengthCorrupt := newMemoryJournalStorage()
	lengthBytes := append([]byte(nil), validBytes...)
	declared := binary.BigEndian.Uint32(lengthBytes[16:20])
	binary.BigEndian.PutUint32(lengthBytes[16:20], declared+1)
	binary.BigEndian.PutUint32(lengthBytes[20:24], crc32.Checksum(lengthBytes[:20], crc32.MakeTable(crc32.Castagnoli)))
	if _, err := lengthCorrupt.WriteAt(lengthBytes, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(lengthCorrupt, crypto); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("complete length corruption did not fail closed: %v", err)
	}
	if got, _ := lengthCorrupt.Size(); got != int64(len(validBytes)) {
		t.Fatal("complete length corruption was truncated")
	}

	impossible := newMemoryJournalStorage()
	if _, err := impossible.WriteAt([]byte{0xff, 0x00}, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(impossible, crypto); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("impossible tail prefix accepted: %v", err)
	}

	poisoned := newMemoryJournalStorage()
	poisoned.setWriteFailure(-1, errors.New("injected writer failure"))
	poisonedWriter, _, err := openJournalWriter(poisoned, crypto)
	if err != nil {
		t.Fatal(err)
	}
	if err := poisonedWriter.Append(record); err == nil {
		t.Fatal("poisoned writer accepted first append")
	}
	if err := poisonedWriter.Append(record); !errors.Is(err, ErrJournalWriterPoisoned) {
		t.Fatalf("poisoned writer accepted another append: %v", err)
	}
}

func TestPhase1TransactionalAppendFailures(t *testing.T) {
	scope := testScope("transactional-append")
	crypto := testJournalCrypto(t, scope)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	firstKey := testMutationKey(scope, "peer", "first")
	secondKey := testMutationKey(scope, "peer", "second")
	first := testIntent(t, crypto, firstKey, testLineage(firstKey, nil), version, 1, "first")
	second := testIntent(t, crypto, secondKey, testLineage(secondKey, nil), version, 2, "second")

	writeStorage := newMemoryJournalStorage()
	writeWriter, _, err := openJournalWriter(writeStorage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeWriter.Append(first); err != nil {
		t.Fatal(err)
	}
	beforeBytes := writeStorage.bytes()
	writeStorage.setWriteFailure(-1, errors.New("write failed"))
	if err := writeWriter.Append(second); err == nil {
		t.Fatal("write failure accepted")
	}
	if got := len(writeWriter.Replay()); got != 1 {
		t.Fatalf("write failure changed replay: %d", got)
	}
	if !bytes.Equal(beforeBytes, writeStorage.bytes()) {
		t.Fatal("write failure changed durable bytes")
	}
	if _, err := writeWriter.reducer.Attempt(secondKey); !errors.Is(err, ErrJournalInvalidKey) {
		t.Fatalf("write failure changed reducer state: %v", err)
	}
	if err := writeWriter.Append(second); !errors.Is(err, ErrJournalWriterPoisoned) {
		t.Fatalf("poisoned writer accepted append: %v", err)
	}
	if _, records, err := openJournalWriter(writeStorage, crypto); err != nil || len(records) != 1 {
		t.Fatalf("write failure reopen records=%d err=%v", len(records), err)
	}

	syncStorage := newMemoryJournalStorage()
	syncWriter, _, err := openJournalWriter(syncStorage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncWriter.Append(first); err != nil {
		t.Fatal(err)
	}
	beforeBytes = syncStorage.bytes()
	syncStorage.setSyncFailure(errors.New("sync failed"))
	if err := syncWriter.Append(second); err == nil {
		t.Fatal("sync failure accepted")
	}
	if got := len(syncWriter.Replay()); got != 1 {
		t.Fatalf("sync failure changed replay: %d", got)
	}
	if !bytes.Equal(beforeBytes, syncStorage.bytes()) {
		t.Fatal("sync failure changed durable bytes")
	}
	if err := syncWriter.Append(second); !errors.Is(err, ErrJournalWriterPoisoned) {
		t.Fatalf("sync-poisoned writer accepted append: %v", err)
	}
	if _, records, err := openJournalWriter(syncStorage, crypto); err != nil || len(records) != 1 {
		t.Fatalf("sync failure reopen records=%d err=%v", len(records), err)
	}
}

func TestPhase1CheckpointPublishCuts(t *testing.T) {
	scope := testScope("checkpoint-cuts")
	crypto := testJournalCrypto(t, scope)
	key := testMutationKey(scope, "peer", "checkpoint")
	lineage := testLineage(key, nil)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	storage := newMemoryJournalStorage()
	journal, err := openJournal(storage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	record := testIntent(t, crypto, key, lineage, version, 1, "checkpoint")
	if err := journal.append(record); err != nil {
		t.Fatal(err)
	}
	beforeBytes := storage.bytes()
	writeErr := errors.New("checkpoint publication failed")
	storage.setCheckpointFailure(writeErr)
	if err := journal.checkpoint(); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("pre-publish failure = %v", err)
	}
	if afterBytes := storage.bytes(); !bytes.Equal(afterBytes, beforeBytes) {
		t.Fatal("pre-publish failure changed journal bytes")
	}
	if marker, _ := storage.ReadJournalGeneration(); len(marker) != 0 {
		t.Fatal("pre-publish failure published a marker")
	}
	markerStorage := newMemoryJournalStorage()
	markerJournal, err := openJournal(markerStorage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	markerKey := testMutationKey(scope, "peer", "marker-failure")
	if err := markerJournal.append(testIntent(t, crypto, markerKey, testLineage(markerKey, nil), version, 1, "marker")); err != nil {
		t.Fatal(err)
	}
	markerBefore := markerStorage.bytes()
	markerStorage.setGenerationFailure(errors.New("marker publication failed"))
	if err := markerJournal.checkpoint(); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("marker publication failure = %v", err)
	}
	if !bytes.Equal(markerBefore, markerStorage.bytes()) {
		t.Fatal("marker publication failure changed old segment")
	}
	if marker, _ := markerStorage.ReadJournalGeneration(); len(marker) != 0 {
		t.Fatal("marker publication failure published active generation")
	}
	markerStorage.setGenerationFailure(nil)
	oldGenerationWriter, oldGenerationRecords, err := openJournalWriter(markerStorage, crypto)
	if err != nil || len(oldGenerationRecords) != 1 || len(oldGenerationWriter.Replay()) != 1 {
		t.Fatalf("pre-marker candidate reopen did not retain old generation: records=%d err=%v", len(oldGenerationRecords), err)
	}
	storage.setCheckpointFailure(nil)
	if err := journal.checkpoint(); err != nil {
		t.Fatal(err)
	}
	if got, _ := storage.Size(); got != 0 {
		t.Fatalf("published checkpoint retained old segment: %d bytes", got)
	}
	markerData, err := storage.ReadJournalGeneration()
	if err != nil || len(markerData) == 0 {
		t.Fatalf("active generation marker missing: %v", err)
	}
	activeMarker, err := unmarshalJournalGenerationMarker(markerData, storage.JournalIdentity())
	if err != nil {
		t.Fatal(err)
	}
	activeCandidate, err := storage.ReadJournalCheckpointGeneration(activeMarker.Generation)
	if err != nil || len(activeCandidate) == 0 {
		t.Fatalf("active candidate missing: %v", err)
	}
	beforeMarkerBytes := storage.bytes()
	if err := storage.WriteJournalCheckpointGeneration(activeMarker.Generation+1, []byte("orphan candidate")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(storage, crypto); err != nil {
		t.Fatalf("orphan candidate changed active generation: %v", err)
	}
	if !bytes.Equal(beforeMarkerBytes, storage.bytes()) {
		t.Fatal("orphan candidate changed the active segment")
	}
	staleMarker := activeMarker
	staleMarker.Generation++
	staleMarkerData, err := marshalJournalGenerationMarker(staleMarker)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteJournalGeneration(staleMarkerData); err != nil {
		t.Fatal(err)
	}
	staleBytes := storage.bytes()
	if _, _, err := openJournalWriter(storage, crypto); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("stale marker/candidate accepted: %v", err)
	}
	if !bytes.Equal(staleBytes, storage.bytes()) {
		t.Fatal("stale marker failure changed active segment")
	}
	if err := storage.WriteJournalGeneration(markerData); err != nil {
		t.Fatal(err)
	}
	foreign := newMemoryJournalStorageWithIdentity(testDigest("foreign-journal"))
	if _, _, err := openJournalWriter(foreign, crypto); err != nil {
		t.Fatal(err)
	}
	if err := foreign.WriteJournalCheckpointGeneration(activeMarker.Generation, activeCandidate); err != nil {
		t.Fatal(err)
	}
	if err := foreign.WriteJournalGeneration(markerData); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(foreign, crypto); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("foreign generation transplant accepted: %v", err)
	}
	badPayload, err := record.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	badFrame, err := marshalJournalFrame(record.GetKind(), activeMarker.NextSequence+1, badPayload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.WriteAt(badFrame, 0); err != nil {
		t.Fatal(err)
	}
	unexplainedBytes := storage.bytes()
	if _, _, err := openJournalWriter(storage, crypto); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("unexplained sequence mismatch accepted: %v", err)
	}
	if !bytes.Equal(unexplainedBytes, storage.bytes()) {
		t.Fatal("unexplained sequence mismatch truncated old segment")
	}
	if err := storage.Truncate(0); err != nil {
		t.Fatal(err)
	}
	key2 := testMutationKey(scope, "peer", "checkpoint-tail")
	if err := journal.append(testIntent(t, crypto, key2, testLineage(key2, nil), version, 2, "tail")); err != nil {
		t.Fatal(err)
	}
	reopened, records, err := openJournalWriter(storage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(reopened.Replay()) != 1 {
		t.Fatalf("checkpoint tail replay = %d", len(records))
	}
	if got := reopened.reducer.Snapshot(); !reflect.DeepEqual(got, journal.writer.reducer.Snapshot()) {
		t.Fatalf("checkpoint live/reopen reducer diverged:\nlive=%#v\nreopen=%#v", journal.writer.reducer.Snapshot(), got)
	}
	if err := journal.checkpoint(); err != nil {
		t.Fatalf("generation-2 checkpoint: %v", err)
	}
	generation2Data, err := storage.ReadJournalGeneration()
	if err != nil {
		t.Fatal(err)
	}
	generation2, err := unmarshalJournalGenerationMarker(generation2Data, storage.JournalIdentity())
	floor2, floorErr := storage.JournalGenerationFloor()
	if err != nil || floorErr != nil || generation2.Generation != activeMarker.Generation+1 || floor2 != generation2.Generation {
		t.Fatalf("generation-2 marker/floor invalid: marker=%d err=%v floorErr=%v floor=%d", generation2.Generation, err, floorErr, floor2)
	}
	generation2Candidate, err := storage.ReadJournalCheckpointGeneration(generation2.Generation)
	if err != nil || len(generation2Candidate) == 0 {
		t.Fatalf("generation-2 candidate missing: %v", err)
	}
	rollbackBytes := storage.bytes()
	if err := storage.WriteJournalGeneration(markerData); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(storage, crypto); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("valid stale generation marker accepted: %v", err)
	}
	if !bytes.Equal(rollbackBytes, storage.bytes()) {
		t.Fatal("stale generation marker failure changed durable journal")
	}
	if err := storage.WriteJournalGeneration(generation2Data); err != nil {
		t.Fatal(err)
	}
	key3 := testMutationKey(scope, "peer", "generation-2-tail")
	if err := journal.append(testIntent(t, crypto, key3, testLineage(key3, nil), version, 3, "generation-2-tail")); err != nil {
		t.Fatal(err)
	}
	storage.setGenerationFailure(errors.New("generation-2 marker publication failed"))
	if err := journal.checkpoint(); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("generation-2 marker failure: %v", err)
	}
	storage.setGenerationFailure(nil)
	reopenedGeneration2, generation2Tail, err := openJournalWriter(storage, crypto)
	if err != nil || len(generation2Tail) != 1 {
		t.Fatalf("generation-2 pre-marker reopen lost tail: records=%d err=%v", len(generation2Tail), err)
	}
	if _, err := reopenedGeneration2.reducer.Attempt(key3); err != nil {
		t.Fatalf("generation-2 tail state missing: %v", err)
	}
	badGeneration2Payload, err := testIntent(t, crypto, key3, testLineage(key3, nil), version, 3, "generation-2-tail").MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	badGeneration2Frame, err := marshalJournalFrame(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, generation2.NextSequence+1, badGeneration2Payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.WriteAt(badGeneration2Frame, 0); err != nil {
		t.Fatal(err)
	}
	generation2CorruptBytes := storage.bytes()
	if _, _, err := openJournalWriter(storage, crypto); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("generation-2 impossible sequence accepted: %v", err)
	}
	if !bytes.Equal(generation2CorruptBytes, storage.bytes()) {
		t.Fatal("generation-2 sequence corruption changed durable bytes")
	}

	syncStorage := newMemoryJournalStorage()
	syncJournal, err := openJournal(syncStorage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	syncKey := testMutationKey(scope, "peer", "checkpoint-sync")
	if err := syncJournal.append(testIntent(t, crypto, syncKey, testLineage(syncKey, nil), version, 1, "sync-cut")); err != nil {
		t.Fatal(err)
	}
	syncStorage.setSyncFailure(errors.New("checkpoint segment sync failed"))
	if err := syncJournal.checkpoint(); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("post-publish sync failure = %v", err)
	}
	if marker, _ := syncStorage.ReadJournalGeneration(); len(marker) == 0 {
		t.Fatal("post-publish sync failure lost published marker")
	}
	syncStorage.setSyncFailure(nil)
	reopenedSync, records, err := openJournalWriter(syncStorage, crypto)
	if err != nil || len(records) != 0 {
		t.Fatalf("reopen after post-publish sync cut: records=%d err=%v", len(records), err)
	}
	if _, err := reopenedSync.reducer.Attempt(syncKey); err != nil {
		t.Fatalf("published checkpoint state was not hydrated: %v", err)
	}
	floorReadStorage := newMemoryJournalStorage()
	floorReadErr := errors.New("generation floor read failed")
	floorReadStorage.setGenerationFloorReadFailure(floorReadErr)
	if _, err := floorReadStorage.JournalGenerationFloor(); !errors.Is(err, floorReadErr) {
		t.Fatalf("generation floor read failure was swallowed: %v", err)
	}
	if _, _, err := openJournalWriter(floorReadStorage, crypto); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("generation floor read failure was accepted: %v", err)
	}

	floorStorage := newMemoryJournalStorage()
	floorJournal, err := openJournal(floorStorage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	floorKey := testMutationKey(scope, "peer", "checkpoint-floor")
	if err := floorJournal.append(testIntent(t, crypto, floorKey, testLineage(floorKey, nil), version, 1, "floor-cut")); err != nil {
		t.Fatal(err)
	}
	floorStorage.setGenerationFloorFailure(errors.New("generation floor publication failed"))
	if err := floorJournal.checkpoint(); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("floor publication failure = %v", err)
	}
	floorMarkerData, err := floorStorage.ReadJournalGeneration()
	floorZero, floorErr := floorStorage.JournalGenerationFloor()
	if err != nil || floorErr != nil || len(floorMarkerData) == 0 || floorZero != 0 {
		t.Fatalf("marker/floor ordering was not observable: marker=%d err=%v floorErr=%v floor=%d", len(floorMarkerData), err, floorErr, floorZero)
	}
	floorStorage.setGenerationFloorFailure(nil)
	repairedFloor, floorRecords, err := openJournalWriter(floorStorage, crypto)
	if err != nil || len(floorRecords) != 0 {
		t.Fatalf("marker-ahead floor repair failed: records=%d err=%v", len(floorRecords), err)
	}
	if err := repairedFloor.activatePending(); err != nil {
		t.Fatal(err)
	}
	floorOne, floorErr := floorStorage.JournalGenerationFloor()
	if floorErr != nil || floorOne != 1 {
		t.Fatalf("reopen did not repair publication floor: floor=%d err=%v", floorOne, floorErr)
	}
	if _, err := repairedFloor.reducer.Attempt(floorKey); err != nil {
		t.Fatalf("floor-repaired checkpoint state was not hydrated: %v", err)
	}
}

func TestPhase1RetiresGenerationTwoTailByDigest(t *testing.T) {
	scope := testScope("generation-two-retire")
	crypto := testJournalCrypto(t, scope)
	storage := newMemoryJournalStorage()
	journal, err := openJournal(storage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	version := JournalVersion(1, 1, 1, testDigest("config"))
	keyOne := testMutationKey(scope, "peer", "generation-one")
	if err := journal.append(testIntent(t, crypto, keyOne, testLineage(keyOne, nil), version, 1, "generation-one")); err != nil {
		t.Fatal(err)
	}
	if err := journal.checkpoint(); err != nil {
		t.Fatal(err)
	}
	keyTwo := testMutationKey(scope, "peer", "generation-two")
	if err := journal.append(testIntent(t, crypto, keyTwo, testLineage(keyTwo, nil), version, 2, "generation-two")); err != nil {
		t.Fatal(err)
	}
	retired := storage.bytes()
	retiredDigest := sha256.Sum256(retired)
	storage.setTruncateFailure(errors.New("pre-truncate crash"))
	if err := journal.checkpoint(); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("pre-truncate checkpoint failure = %v", err)
	}
	markerData, err := storage.ReadJournalGeneration()
	if err != nil {
		t.Fatal(err)
	}
	marker, err := unmarshalJournalGenerationMarker(markerData, storage.JournalIdentity())
	if err != nil {
		t.Fatal(err)
	}
	floor, floorErr := storage.JournalGenerationFloor()
	if floorErr != nil || floor != 2 || marker.Generation != 2 || marker.NextSequence != 3 ||
		marker.RetiredLength != uint64(len(retired)) || !bytes.Equal(marker.RetiredDigest, retiredDigest[:]) {
		t.Fatalf("pre-truncate marker mismatch: marker=%#v floor=%d floorErr=%v", marker, floor, floorErr)
	}
	storage.setTruncateFailure(nil)
	reopened, records, err := openJournalWriter(storage, crypto)
	if err != nil || len(records) != 0 {
		t.Fatalf("generation-2 retired tail was not recovered: records=%d err=%v", len(records), err)
	}
	if err := reopened.activatePending(); err != nil {
		t.Fatal(err)
	}
	if size, sizeErr := storage.Size(); sizeErr != nil || size != 0 {
		t.Fatalf("retired tail remained after exact validation: size=%d err=%v", size, sizeErr)
	}
	if _, err := reopened.reducer.Attempt(keyTwo); err != nil {
		t.Fatalf("generation-2 checkpoint state was lost: %v", err)
	}
}

func TestPhase1FileJournalMetadataLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journal")
	storage, err := OpenFileJournalStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	identity := storage.JournalIdentity()
	floor, floorErr := storage.JournalGenerationFloor()
	if floorErr != nil || floor != 0 {
		t.Fatalf("new storage floor = %d, err=%v", floor, floorErr)
	}
	if marker, markerErr := storage.ReadJournalGeneration(); markerErr != nil || len(marker) != 0 {
		t.Fatalf("new storage marker = %d bytes, err=%v", len(marker), markerErr)
	}
	if _, err := os.Stat(path + ".identity"); err != nil {
		t.Fatalf("identity sidecar was not initialized: %v", err)
	}
	if _, err := os.Stat(path + ".generation.floor"); err != nil {
		t.Fatalf("generation floor sidecar was not initialized: %v", err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFileJournalStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(identity, reopened.JournalIdentity()) {
		t.Fatal("journal identity changed across reopen")
	}
	if floor, floorErr := reopened.JournalGenerationFloor(); floorErr != nil || floor != 0 {
		t.Fatalf("reopened storage floor = %d, err=%v", floor, floorErr)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	emptyPath := filepath.Join(dir, "existing-empty")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFileJournalStorage(emptyPath); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("existing empty journal without identity accepted: %v", err)
	}

	identityOnlyPath := filepath.Join(dir, "identity-only")
	if err := os.WriteFile(identityOnlyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identityOnlyPath+".identity", identity, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFileJournalStorage(identityOnlyPath); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("existing empty journal without floor accepted: %v", err)
	}

	crypto := testJournalCrypto(t, testScope("file-storage"))
	storage, err = OpenFileJournalStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := openJournal(storage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	key := testMutationKey(testScope("file-storage"), "peer", "marker")
	version := JournalVersion(1, 1, 1, testDigest("config"))
	intent, err := NewJournalIntent(key, testLineage(key, nil), version, []byte("marker"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := NewJournalIntentRecord(crypto, 1, intent, SOJournalReadiness_SO_JOURNAL_READINESS_READY, storage.JournalIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.append(record); err != nil {
		t.Fatal(err)
	}
	if err := journal.checkpoint(); err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path + ".generation"); err != nil {
		t.Fatal(err)
	}
	reopened, err = OpenFileJournalStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := openJournalWriter(reopened, crypto); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("checkpoint marker loss accepted: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path + ".generation.floor"); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFileJournalStorage(path); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("checkpoint floor loss accepted: %v", err)
	}
}

func prepareRetainedCheckpoint(t *testing.T, markerAhead bool, lookupState SOReceiptState) (*memoryJournalStorage, *JournalCrypto, *SOMutationKey, []byte, uint64, []byte) {
	t.Helper()
	scope := testScope("activation-boundary")
	crypto := testJournalCrypto(t, scope)
	storage := newMemoryJournalStorage()
	pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}
	key := testMutationKey(scope, "peer", "activation")
	lineage := testLineage(key, nil)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	intentRecord := testIntent(t, crypto, key, lineage, version, 1, "activation")
	if err := pipeline.appendRecord(intentRecord); err != nil {
		t.Fatal(err)
	}
	intent := testDecodedIntent(t, crypto, intentRecord, key, lineage, version, 1)
	envelopeBytes := []byte("activation-envelope")
	envelope, err := NewJournalEnvelopeRecord(crypto, 2, intent, envelopeBytes, journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(envelope); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(newJournalSentRecord(key, lineage, version)); err != nil {
		t.Fatal(err)
	}
	envelopeDigest := sha256.Sum256(envelopeBytes)
	receipt := testReceipt(key, envelopeDigest[:], []byte("activation-terminal"), 1, testDigest("activation-root"))
	if lookupState != 0 {
		response := []byte("activation-lookup")
		lookup := &SOJournalLookup{
			Key:               key.CloneVT(),
			State:             lookupState,
			Response:          response,
			ResponseDigest:    testDigest(string(response)),
			ConfigChainDigest: testDigest("config"),
		}
		if err := pipeline.appendRecord(NewJournalReceiptLookupRecord(key, lineage, version, lookup)); err != nil {
			t.Fatal(err)
		}
		if lookupState == SOReceiptState_SO_RECEIPT_STATE_NO_RECORD {
			if err := pipeline.AppendResendAuthorization(key); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := pipeline.AppendReceipt(key, receipt); err != nil {
		t.Fatal(err)
	}
	projection := &SOJournalProjection{
		Key:                     key.CloneVT(),
		ReceiptDigest:           receipt.GetTerminalReceiptDigest(),
		AuthoritativeRootSeqno:  1,
		AuthoritativeRootDigest: testDigest("activation-root"),
	}
	if err := pipeline.AppendProjection(key, projection); err != nil {
		t.Fatal(err)
	}
	oldBytes := storage.bytes()
	if markerAhead {
		storage.setGenerationFloorFailure(errors.New("activation floor publication failed"))
	} else {
		storage.setTruncateFailure(errors.New("activation truncate failed"))
	}
	if _, err := pipeline.Checkpoint(key); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("retained checkpoint setup error = %v", err)
	}
	storage.setGenerationFloorFailure(nil)
	storage.setTruncateFailure(nil)
	marker, err := storage.ReadJournalGeneration()
	if err != nil {
		t.Fatal(err)
	}
	floor, err := storage.JournalGenerationFloor()
	if err != nil {
		t.Fatal(err)
	}
	return storage, crypto, key, marker, floor, oldBytes
}

func TestPhase1ActivationVerificationBoundary(t *testing.T) {
	for _, fixture := range []struct {
		name             string
		markerAhead      bool
		lookupState      SOReceiptState
		wantVerification error
	}{
		{name: "receipt-marker-ahead", markerAhead: true, wantVerification: ErrJournalReceiptVerification},
		{name: "receipt-marker-active", wantVerification: ErrJournalReceiptVerification},
		{name: "no-record-marker-ahead", markerAhead: true, lookupState: SOReceiptState_SO_RECEIPT_STATE_NO_RECORD, wantVerification: ErrJournalLookupVerification},
		{name: "no-record-marker-active", lookupState: SOReceiptState_SO_RECEIPT_STATE_NO_RECORD, wantVerification: ErrJournalLookupVerification},
		{name: "pending-marker-ahead", markerAhead: true, lookupState: SOReceiptState_SO_RECEIPT_STATE_PENDING, wantVerification: ErrJournalLookupVerification},
		{name: "pending-marker-active", lookupState: SOReceiptState_SO_RECEIPT_STATE_PENDING, wantVerification: ErrJournalLookupVerification},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			storage, crypto, _, markerBefore, floorBefore, oldBytes := prepareRetainedCheckpoint(t, fixture.markerAhead, fixture.lookupState)
			rejectReceipt := JournalReceiptVerifierFunc(func(*SOJournalReceipt, *SOJournalVersionTuple) error {
				return errors.New("activation receipt authority rejected")
			})
			rejectLookup := JournalLookupVerifierFunc(func(*SOJournalLookup, *SOJournalVersionTuple) error {
				return errors.New("activation lookup authority rejected")
			})
			receiptVerifier := testReceiptVerifier()
			lookupVerifier := testLookupVerifier()
			if fixture.lookupState != 0 {
				lookupVerifier = rejectLookup
			} else {
				receiptVerifier = rejectReceipt
			}
			if _, err := OpenJournalPipelineWithCrypto(storage, crypto, receiptVerifier, lookupVerifier); !errors.Is(err, fixture.wantVerification) {
				t.Fatalf("reopen crossed rejected activation: %v", err)
			}
			if !bytes.Equal(storage.bytes(), oldBytes) {
				t.Fatal("rejected activation changed retired journal bytes")
			}
			markerAfter, err := storage.ReadJournalGeneration()
			if err != nil || !bytes.Equal(markerAfter, markerBefore) {
				t.Fatalf("rejected activation changed marker: before=%x after=%x err=%v", markerBefore, markerAfter, err)
			}
			floorAfter, err := storage.JournalGenerationFloor()
			if err != nil || floorAfter != floorBefore {
				t.Fatalf("rejected activation changed floor: before=%d after=%d err=%v", floorBefore, floorAfter, err)
			}
		})
	}
	for _, markerAhead := range []bool{true, false} {
		t.Run("success-"+map[bool]string{true: "marker-ahead", false: "marker-active"}[markerAhead], func(t *testing.T) {
			storage, crypto, _, markerBefore, floorBefore, _ := prepareRetainedCheckpoint(t, markerAhead, 0)
			if _, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier()); err != nil {
				t.Fatal(err)
			}
			if size, err := storage.Size(); err != nil || size != 0 {
				t.Fatalf("successful activation retained old bytes: size=%d err=%v", size, err)
			}
			markerAfter, err := storage.ReadJournalGeneration()
			if err != nil || !bytes.Equal(markerAfter, markerBefore) {
				t.Fatalf("successful activation changed marker: %v", err)
			}
			floorAfter, err := storage.JournalGenerationFloor()
			wantFloor := floorBefore
			if markerAhead {
				wantFloor++
			}
			if err != nil || floorAfter != wantFloor {
				t.Fatalf("successful activation floor=%d want=%d err=%v", floorAfter, wantFloor, err)
			}
		})
	}
}
func TestPhase1FileJournalInitializationCuts(t *testing.T) {
	identity := testDigest("initialization-identity")
	for _, fixture := range []struct {
		name        string
		initialized bool
		file        bool
		floor       bool
		marker      bool
		nonempty    bool
		wantOpen    bool
	}{
		{name: "identity-before-file", wantOpen: true},
		{name: "file-before-floor", file: true, wantOpen: true},
		{name: "floor-before-finalize", file: true, floor: true, wantOpen: true},
		{name: "initializing-nonempty", file: true, nonempty: true},
		{name: "initializing-marker", file: true, floor: true, marker: true},
		{name: "initialized-file-missing", initialized: true},
		{name: "initialized-floor-missing", initialized: true, file: true},
		{name: "initialized-complete", initialized: true, file: true, floor: true, wantOpen: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "journal")
			if err := writeJournalIdentityMetadata(path, identity, fixture.initialized); err != nil {
				t.Fatal(err)
			}
			if fixture.file {
				data := []byte(nil)
				if fixture.nonempty {
					data = []byte("unexpected durable bytes")
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if fixture.floor {
				floor, err := marshalJournalGenerationFloor(identity, 0)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path+".generation.floor", floor, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if fixture.marker {
				if err := os.WriteFile(path+".generation", []byte{1}, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			storage, err := OpenFileJournalStorage(path)
			if fixture.wantOpen {
				if err != nil {
					t.Fatal(err)
				}
				if err := storage.Close(); err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrJournalCheckpointCorrupt) {
				t.Fatalf("invalid initialization cut accepted: %v", err)
			}
		})
	}
}

func TestPhase1ResendAuthorizationTerminalClears(t *testing.T) {
	setup := func(t *testing.T) (*JournalPipeline, *SOMutationKey, *JournalCrypto, *SOJournalVersionTuple) {
		t.Helper()
		scope := testScope("resend-terminal")
		crypto := testJournalCrypto(t, scope)
		pipeline := testPipeline(t, crypto)
		key := testMutationKey(scope, "peer", "resend")
		lineage := testLineage(key, nil)
		version := JournalVersion(1, 1, 1, testDigest("config"))
		intentRecord := testIntent(t, crypto, key, lineage, version, 1, "resend")
		if err := pipeline.appendRecord(intentRecord); err != nil {
			t.Fatal(err)
		}
		intent := testDecodedIntent(t, crypto, intentRecord, key, lineage, version, 1)
		envelopeBytes := []byte("resend-envelope")
		envelope, err := NewJournalEnvelopeRecord(crypto, 2, intent, envelopeBytes, journalDefaultIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if err := pipeline.appendRecord(envelope); err != nil {
			t.Fatal(err)
		}
		if err := pipeline.appendRecord(newJournalSentRecord(key, lineage, version)); err != nil {
			t.Fatal(err)
		}
		response := []byte("resend-no-record")
		lookup := &SOJournalLookup{
			Key:               key.CloneVT(),
			State:             SOReceiptState_SO_RECEIPT_STATE_NO_RECORD,
			Response:          response,
			ResponseDigest:    testDigest(string(response)),
			ConfigChainDigest: testDigest("config"),
		}
		if err := pipeline.AppendReceiptLookup(key, lookup); err != nil {
			t.Fatal(err)
		}
		if err := pipeline.AppendResendAuthorization(key); err != nil {
			t.Fatal(err)
		}
		attempt, err := pipeline.reducer.Attempt(key)
		if err != nil {
			t.Fatal(err)
		}
		if !attempt.ResendAuthorized {
			t.Fatal("resend authorization was not durable before terminal transition")
		}
		return pipeline, key, crypto, version
	}

	t.Run("delayed-receipt", func(t *testing.T) {
		pipeline, key, crypto, _ := setup(t)
		attempt, err := pipeline.reducer.Attempt(key)
		if err != nil {
			t.Fatal(err)
		}
		envelopeDigest := attempt.EnvelopeDigest
		receipt := testReceipt(key, envelopeDigest, []byte("delayed-terminal"), 1, testDigest("delayed-root"))
		if err := pipeline.AppendReceipt(key, receipt); err != nil {
			t.Fatal(err)
		}
		projection := &SOJournalProjection{
			Key:                     key.CloneVT(),
			ReceiptDigest:           receipt.GetTerminalReceiptDigest(),
			AuthoritativeRootSeqno:  1,
			AuthoritativeRootDigest: testDigest("delayed-root"),
		}
		if err := pipeline.AppendProjection(key, projection); err != nil {
			t.Fatal(err)
		}
		if _, err := pipeline.Checkpoint(key); err != nil {
			t.Fatal(err)
		}
		reopened, err := OpenJournalPipelineWithCrypto(pipeline.journal.writer.storage, crypto, testReceiptVerifier(), testLookupVerifier())
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := reopened.reducer.Attempt(key)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.ResendAuthorized || snapshot.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE {
			t.Fatalf("delayed terminal receipt retained resend authority: %#v", snapshot)
		}
	})

	t.Run("stale-epoch", func(t *testing.T) {
		pipeline, key, crypto, _ := setup(t)
		if err := pipeline.BeforeSend(key, 2, func() error {
			t.Fatal("stale epoch invoked transport callback")
			return nil
		}); !errors.Is(err, ErrJournalStaleTransformEpoch) {
			t.Fatalf("stale epoch result = %v", err)
		}
		if err := pipeline.journal.checkpoint(); err != nil {
			t.Fatal(err)
		}
		reopened, err := OpenJournalPipelineWithCrypto(pipeline.journal.writer.storage, crypto, testReceiptVerifier(), testLookupVerifier())
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := reopened.reducer.Attempt(key)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.ResendAuthorized || snapshot.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH {
			t.Fatalf("stale transition retained resend authority: %#v", snapshot)
		}
	})

	t.Run("recovery-blocked", func(t *testing.T) {
		pipeline, key, crypto, _ := setup(t)
		if err := pipeline.AppendRecoveryBlocked(key, SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_KEY_UNAVAILABLE); err != nil {
			t.Fatal(err)
		}
		if err := pipeline.journal.checkpoint(); err != nil {
			t.Fatal(err)
		}
		reopened, err := OpenJournalPipelineWithCrypto(pipeline.journal.writer.storage, crypto, testReceiptVerifier(), testLookupVerifier())
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := reopened.reducer.Attempt(key)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.ResendAuthorized || snapshot.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED {
			t.Fatalf("recovery transition retained resend authority: %#v", snapshot)
		}
	})
}

func TestPhase1EligibleCheckpointReopenDecrypt(t *testing.T) {
	scope := testScope("eligible-checkpoint")
	crypto := testJournalCrypto(t, scope)
	key := testMutationKey(scope, "peer", "eligible")
	lineage := testLineage(key, nil)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	storage := newMemoryJournalStorage()
	pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}
	intentRecord := testIntent(t, crypto, key, lineage, version, 1, "eligible-operation")
	if err := pipeline.appendRecord(intentRecord); err != nil {
		t.Fatal(err)
	}
	intent := testDecodedIntent(t, crypto, intentRecord, key, lineage, version, 1)
	envelopeBytes := []byte("eligible-envelope")
	envelope, err := NewJournalEnvelopeRecord(crypto, 2, intent, envelopeBytes, journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(envelope); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(newJournalSentRecord(key, lineage, version)); err != nil {
		t.Fatal(err)
	}
	receipt := testReceipt(key, testDigest(string(envelopeBytes)), []byte("eligible-terminal"), 1, testDigest("eligible-root"))
	if err := pipeline.AppendReceipt(key, receipt); err != nil {
		t.Fatal(err)
	}
	acknowledgement := &SOJournalAcknowledgement{Key: key.CloneVT(), ReceiptDigest: receipt.GetTerminalReceiptDigest(), AcknowledgedUnixMillis: 1}
	if err := pipeline.AppendAcknowledgement(key, acknowledgement); err != nil {
		t.Fatal(err)
	}
	projection := &SOJournalProjection{Key: key.CloneVT(), ReceiptDigest: receipt.GetTerminalReceiptDigest(), AuthoritativeRootSeqno: 1, AuthoritativeRootDigest: testDigest("eligible-root")}
	if err := pipeline.AppendProjection(key, projection); err != nil {
		t.Fatal(err)
	}
	live := pipeline.Snapshot()
	if _, err := pipeline.Checkpoint(key); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot(); !reflect.DeepEqual(got, live) {
		t.Fatalf("eligible compact snapshot diverged:\nlive=%#v\nreopen=%#v", live, got)
	}
	rejectReceipt := JournalReceiptVerifierFunc(func(*SOJournalReceipt, *SOJournalVersionTuple) error {
		return errors.New("compact receipt authority rejected")
	})
	if _, err := OpenJournalPipelineWithCrypto(storage, crypto, rejectReceipt, testLookupVerifier()); !errors.Is(err, ErrJournalReceiptVerification) {
		t.Fatalf("compact reopen accepted rejected receipt: %v", err)
	}
	material, err := reopened.Recover(key)
	if err != nil {
		t.Fatal(err)
	}
	if material == nil || !bytes.Equal(material.Envelope, envelopeBytes) || material.Intent == nil {
		t.Fatalf("retained compact material did not decrypt: %#v", material)
	}
}

func TestPhase1MalformedCompactSnapshotRejected(t *testing.T) {
	scope := testScope("malformed-snapshot")
	crypto := testJournalCrypto(t, scope)
	key := testMutationKey(scope, "peer", "snapshot")
	lineage := testLineage(key, nil)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	pipeline := testPipeline(t, crypto)
	intentRecord := testIntent(t, crypto, key, lineage, version, 1, "snapshot")
	if err := pipeline.appendRecord(intentRecord); err != nil {
		t.Fatal(err)
	}
	intent := testDecodedIntent(t, crypto, intentRecord, key, lineage, version, 1)
	envelope, err := NewJournalEnvelopeRecord(crypto, 2, intent, []byte("snapshot-envelope"), journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(envelope); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(newJournalSentRecord(key, lineage, version)); err != nil {
		t.Fatal(err)
	}
	receipt := testReceipt(key, testDigest("snapshot-envelope"), []byte("snapshot-terminal"), 1, testDigest("snapshot-root"))
	if err := pipeline.AppendReceipt(key, receipt); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.AppendProjection(key, &SOJournalProjection{Key: key.CloneVT(), ReceiptDigest: receipt.GetTerminalReceiptDigest(), AuthoritativeRootSeqno: 1, AuthoritativeRootDigest: testDigest("snapshot-root")}); err != nil {
		t.Fatal(err)
	}
	base := pipeline.Snapshot()[0]
	cases := []struct {
		name string
		edit func(*JournalAttemptSnapshot)
	}{
		{name: "nil-key", edit: func(snapshot *JournalAttemptSnapshot) { snapshot.Key = nil }},
		{name: "nil-intent", edit: func(snapshot *JournalAttemptSnapshot) { snapshot.Intent = nil }},
		{name: "history-without-lookup", edit: func(snapshot *JournalAttemptSnapshot) { snapshot.LookupHistory = []*SOJournalLookup{nil} }},
		{name: "eligibility-mismatch", edit: func(snapshot *JournalAttemptSnapshot) { snapshot.CheckpointEligible = false }},
		{name: "projection-receipt-mismatch", edit: func(snapshot *JournalAttemptSnapshot) { snapshot.Projection.ReceiptDigest = testDigest("wrong") }},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			snapshot := cloneJournalAttempt(base)
			fixture.edit(snapshot)
			if err := validateCheckpointAttempt(snapshot); !errors.Is(err, ErrJournalCheckpointCorrupt) {
				t.Fatalf("malformed snapshot accepted: %v", err)
			}
		})
	}
	if err := validateCheckpointAttempt(nil); !errors.Is(err, ErrJournalCheckpointCorrupt) {
		t.Fatalf("nil compact snapshot accepted: %v", err)
	}
}

func TestPhase1JournalCryptoCorpus(t *testing.T) {
	scope := testScope("crypto")
	key := testMutationKey(scope, "peer", "local")
	master := testDigest("master")
	crypto := testJournalCryptoWithMaster(t, scope, master)
	plaintext := []byte("canonical body-neutral operation")
	sealed, err := crypto.SealWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, 1, key, journalDefaultIdentity(), plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed.GetCiphertext(), plaintext) || bytes.Contains(sealed.GetCiphertext(), master) {
		t.Fatal("staged payload or raw key was durable in ciphertext")
	}
	storage := newMemoryJournalStorage()
	journal, err := openJournal(storage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	intentRecord := testIntent(t, crypto, key, testLineage(key, nil), JournalVersion(1, 1, 1, testDigest("config")), 1, string(plaintext))
	if err := journal.append(intentRecord); err != nil {
		t.Fatal(err)
	}
	durable := storage.bytes()
	if bytes.Contains(durable, plaintext) || bytes.Contains(durable, master) {
		t.Fatal("durable journal frame contains staged plaintext or raw key")
	}
	opened, err := crypto.OpenWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, 1, key, journalDefaultIdentity(), sealed)

	otherScopeCrypto := testJournalCrypto(t, testScope("other-scope"))
	if _, err := otherScopeCrypto.SealWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, 1, key, journalDefaultIdentity(), plaintext); !errors.Is(err, ErrJournalScopeMismatch) {
		t.Fatalf("cross-scope key accepted: %v", err)
	}
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("decrypt intent: %v", err)
	}
	otherKey := testMutationKey(scope, "other-peer", "local")
	if _, err := crypto.OpenWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, 1, otherKey, journalDefaultIdentity(), sealed); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("ciphertext transplant accepted: %v", err)
	}
	if _, err := crypto.OpenWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE, 1, key, journalDefaultIdentity(), sealed); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("domain-swapped ciphertext accepted: %v", err)
	}
	if _, err := crypto.OpenWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, 2, key, journalDefaultIdentity(), sealed); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("sequence-swapped ciphertext accepted: %v", err)
	}

	unavailable, err := NewJournalCrypto(scope, JournalKeyAuthorityFunc(func([]byte) ([]byte, error) { return nil, ErrJournalKeyUnavailable }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unavailable.SealWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, 1, key, journalDefaultIdentity(), plaintext); !errors.Is(err, ErrJournalKeyUnavailable) {
		t.Fatalf("unavailable key did not fail typed: %v", err)
	}
	authorityFailure, err := NewJournalCrypto(scope, JournalKeyAuthorityFunc(func([]byte) ([]byte, error) { return nil, errors.New("authority denied") }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authorityFailure.SealWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, 1, key, journalDefaultIdentity(), plaintext); !errors.Is(err, ErrJournalKeyAuthority) {
		t.Fatalf("authority failure did not fail typed: %v", err)
	}

	keyLoss, err := NewJournalCrypto(scope, JournalKeyAuthorityFunc(func([]byte) ([]byte, error) {
		return nil, ErrJournalKeyUnavailable
	}))
	if err != nil {
		t.Fatal(err)
	}
	beforeLoss := storage.bytes()
	if _, err := OpenJournalPipelineWithCrypto(storage, keyLoss, testReceiptVerifier(), testLookupVerifier()); !errors.Is(err, ErrJournalKeyUnavailable) {
		t.Fatalf("key-loss reopen was not typed: %v", err)
	}
	if afterLoss := storage.bytes(); !bytes.Equal(afterLoss, beforeLoss) {
		t.Fatal("key-loss reopen changed durable journal bytes")
	}
}

func TestPhase1Q3aAndLiveReplayEquality(t *testing.T) {
	scope := testScope("q3a")
	crypto := testJournalCrypto(t, scope)
	key := testMutationKey(scope, "peer", "local")
	lineage := testLineage(key, nil)
	oldVersion := JournalVersion(1, 1, 7, testDigest("config-old"))
	storage := newMemoryJournalStorage()
	pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}
	intent := testIntent(t, crypto, key, lineage, oldVersion, 1, "operation")
	if err := pipeline.appendRecord(intent); err != nil {
		t.Fatal(err)
	}
	decoded := testDecodedIntent(t, crypto, intent, key, lineage, oldVersion, 1)
	envelope, err := NewJournalEnvelopeRecord(crypto, 2, decoded, []byte("old-envelope"), journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(envelope); err != nil {
		t.Fatal(err)
	}
	sendCount := 0
	if err := pipeline.BeforeSend(key, 8, func() error { sendCount++; return nil }); !errors.Is(err, ErrJournalStaleTransformEpoch) {
		t.Fatalf("epoch mismatch was not typed: %v", err)
	}
	if sendCount != 0 {
		t.Fatal("old-epoch envelope crossed send boundary")
	}
	attempt, err := pipeline.reducer.Attempt(key)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH {
		t.Fatalf("attempt state = %v, want stale epoch", attempt.State)
	}
	if err := pipeline.appendRecord(NewJournalLineageRecoveryBlockedRecord(key, lineage, oldVersion)); err != nil {
		t.Fatal(err)
	}
	if staleAttempt, err := pipeline.reducer.Attempt(key); err != nil {
		t.Fatal(err)
	} else if staleAttempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH || !staleAttempt.LineageRecoveryBlocked {
		t.Fatalf("stale lineage block was not retained: %#v", staleAttempt)
	}
	blocked := testPipeline(t, crypto)
	blockedKey := testMutationKey(scope, "peer", "local-blocked")
	blockedLineage := testLineage(blockedKey, nil)
	blockedVersion := JournalVersion(1, 1, 7, testDigest("config-old"))
	blockedIntent := testIntent(t, crypto, blockedKey, blockedLineage, blockedVersion, 1, "operation-blocked")
	if err := blocked.appendRecord(blockedIntent); err != nil {
		t.Fatal(err)
	}
	blockedDecoded := testDecodedIntent(t, crypto, blockedIntent, blockedKey, blockedLineage, blockedVersion, 1)
	blockedEnvelope, err := NewJournalEnvelopeRecord(crypto, 2, blockedDecoded, []byte("blocked-envelope"), journalDefaultIdentity())
	if err != nil {

		t.Fatal(err)
	}
	if err := blocked.appendRecord(blockedEnvelope); err != nil {
		t.Fatal(err)
	}
	if err := blocked.appendRecord(NewJournalRecoveryBlockedRecord(blockedKey, blockedLineage, blockedVersion, SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_KEY_UNAVAILABLE)); err != nil {
		t.Fatal(err)
	}
	if blockedAttempt, err := blocked.reducer.Attempt(blockedKey); err != nil {
		t.Fatal(err)
	} else if blockedAttempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED {
		t.Fatalf("authority-loss state = %v, want recovery blocked", blockedAttempt.State)
	}

	newKey := testMutationKey(scope, "peer", "local-successor")
	newVersion := JournalVersion(2, 1, 8, testDigest("config-new"))
	successor, err := pipeline.NewSuccessor(key, newKey, newVersion, []byte("operation-current"))
	if err != nil {
		t.Fatal(err)
	}
	if !successor.GetLineage().GetSupersedes().EqualExact(key) || successor.GetKey().EqualExact(key) {
		t.Fatal("successor did not carry a fresh exact key and immutable supersedes link")
	}
	if err := pipeline.appendRecord(testIntent(t, crypto, newKey, successor.GetLineage(), newVersion, uint64(len(pipeline.Replay())+1), "operation-current")); err != nil {
		t.Fatal(err)
	}

	live := pipeline.Snapshot()
	replayed, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}
	if got := replayed.Snapshot(); !reflect.DeepEqual(got, live) {
		t.Fatalf("live/replay reduction diverged:\nlive=%#v\nreplay=%#v", live, got)
	}
}

func TestPhase1AmbiguousSendRequiresLookup(t *testing.T) {
	scope := testScope("ambiguous-send")
	crypto := testJournalCrypto(t, scope)
	key := testMutationKey(scope, "peer", "ambiguous")
	lineage := testLineage(key, nil)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	storage := newMemoryJournalStorage()
	pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}
	intent := testIntent(t, crypto, key, lineage, version, 1, "ambiguous")
	if err := pipeline.appendRecord(intent); err != nil {
		t.Fatal(err)
	}
	decoded := testDecodedIntent(t, crypto, intent, key, lineage, version, 1)
	envelope, err := NewJournalEnvelopeRecord(crypto, 2, decoded, []byte("ambiguous-envelope"), journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(envelope); err != nil {
		t.Fatal(err)
	}
	sendCount := 0
	sendErr := errors.New("ambiguous transport result")
	if err := pipeline.BeforeSend(key, 1, func() error { sendCount++; return sendErr }); !errors.Is(err, sendErr) {
		t.Fatalf("ambiguous send error = %v", err)
	}
	if sendCount != 1 {
		t.Fatalf("send count = %d, want 1", sendCount)
	}
	reopened, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.BeforeSend(key, 1, func() error { sendCount++; return nil }); !errors.Is(err, ErrJournalLookupRequired) {
		t.Fatalf("recovered send crossed boundary without lookup: %v", err)
	}
	if sendCount != 1 {
		t.Fatalf("recovered send count = %d, want 1", sendCount)
	}
	lookup := &SOJournalLookup{Key: key.CloneVT(), State: SOReceiptState_SO_RECEIPT_STATE_NO_RECORD, Response: []byte("no-record"), ResponseDigest: testDigest("no-record"), ConfigChainDigest: testDigest("config")}
	if err := reopened.appendRecord(NewJournalReceiptLookupRecord(key, lineage, version, lookup)); err != nil {
		t.Fatal(err)
	}
	if err := reopened.appendRecord(NewJournalResendAuthorizedRecord(key, lineage, version)); err != nil {
		t.Fatal(err)
	}
	if err := reopened.BeforeSend(key, 1, func() error { sendCount++; return nil }); err != nil {
		t.Fatal(err)
	}
	if sendCount != 2 {
		t.Fatalf("authorized resend count = %d, want 2", sendCount)
	}
}

func TestPhase1BodyNeutralFixtures(t *testing.T) {
	scope := testScope("fixtures")
	crypto := testJournalCrypto(t, scope)
	mapData, err := (&SORootInner{Seqno: 7, StateData: []byte("map-state")}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	counterData, err := (&SOOperationRef{PeerId: "counter-peer", Nonce: 11}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	mapInner, err := (&SOOperationInner{PeerId: "map-peer", LocalId: "01hfixturemap00000000000000", Nonce: 1, OpData: mapData}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	counterInner, err := (&SOOperationInner{PeerId: "counter-peer", LocalId: "01hfixturecounter000000000", Nonce: 2, OpData: counterData}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	mapPayload, err := (&SOOperation{Inner: mapInner}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	counterPayload, err := (&SOOperation{Inner: counterInner}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name    string
		payload []byte
		opData  []byte
	}{
		{name: "world-neutral", payload: mapPayload, opData: mapData},
		{name: "non-world", payload: counterPayload, opData: counterData},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			key := testMutationKey(scope, fixture.name, "same-contract")
			lineage := testLineage(key, nil)
			version := JournalVersion(1, 1, 1, testDigest("config"))
			intent := testIntent(t, crypto, key, lineage, version, 1, string(fixture.payload))
			pipeline := testPipeline(t, crypto)
			if err := pipeline.appendRecord(intent); err != nil {
				t.Fatal(err)
			}
			if got := pipeline.Replay(); len(got) != 1 || !bytes.Equal(got[0].GetKey().GetOriginScopeId(), scope) {
				t.Fatalf("body-neutral replay lost exact key: %#v", got)
			}
			decoded := testDecodedIntent(t, crypto, intent, key, lineage, version, 1)
			operation := new(SOOperation)
			if err := operation.UnmarshalVT(decoded.GetCanonicalOperation()); err != nil {
				t.Fatal(err)
			}
			inner := new(SOOperationInner)
			if err := inner.UnmarshalVT(operation.GetInner()); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(inner.GetOpData(), fixture.opData) {
				t.Fatalf("operation payload changed: got %x want %x", inner.GetOpData(), fixture.opData)
			}
		})
	}
}

func testPipeline(t *testing.T, crypto *JournalCrypto) *JournalPipeline {
	t.Helper()
	pipeline, err := OpenJournalPipelineWithCrypto(newMemoryJournalStorage(), crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}
	return pipeline
}

func TestPhase1ReceiptVerifierBoundary(t *testing.T) {
	scope := testScope("receipt-verifier")
	crypto := testJournalCrypto(t, scope)
	key := testMutationKey(scope, "peer", "local")
	lineage := testLineage(key, nil)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	storage := newMemoryJournalStorage()
	if _, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), nil); !errors.Is(err, ErrJournalLookupVerifierRequired) {
		t.Fatalf("nil receipt verifier accepted: %v", err)
	}
	failed := JournalLookupVerifierFunc(func(*SOJournalLookup, *SOJournalVersionTuple) error {
		return errors.New("lookup response rejected")
	})
	pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), failed)
	if err != nil {
		t.Fatal(err)
	}
	intent := testIntent(t, crypto, key, lineage, version, 1, "receipt-verifier")
	if err := pipeline.appendRecord(intent); err != nil {
		t.Fatal(err)
	}
	decoded := testDecodedIntent(t, crypto, intent, key, lineage, version, 1)
	envelope, err := NewJournalEnvelopeRecord(crypto, 2, decoded, []byte("envelope"), journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(envelope); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.appendRecord(newJournalSentRecord(key, lineage, version)); err != nil {
		t.Fatal(err)
	}
	lookup := &SOJournalLookup{Key: key.CloneVT(), State: SOReceiptState_SO_RECEIPT_STATE_NO_RECORD, Response: []byte("no-record"), ResponseDigest: testDigest("no-record"), ConfigChainDigest: testDigest("config")}
	if err := pipeline.appendRecord(NewJournalReceiptLookupRecord(key, lineage, version, lookup)); !errors.Is(err, ErrJournalLookupVerification) {
		t.Fatalf("failed lookup verifier accepted append: %v", err)
	}
	if got := len(pipeline.Replay()); got != 3 {
		t.Fatalf("failed lookup append changed journal: %d records", got)
	}
	receipt := testReceipt(key, testDigest("envelope"), []byte("terminal"), 1, testDigest("root"))
	if err := pipeline.AppendReceipt(key, receipt); err != nil {
		t.Fatal(err)
	}
	rejectReceipt := JournalReceiptVerifierFunc(func(*SOJournalReceipt, *SOJournalVersionTuple) error {
		return errors.New("receipt authority rejected")
	})
	if _, err := OpenJournalPipelineWithCrypto(storage, crypto, rejectReceipt, testLookupVerifier()); !errors.Is(err, ErrJournalReceiptVerification) {
		t.Fatalf("reopen accepted rejected terminal receipt: %v", err)
	}
}

func TestPhase1LookupVerifierReopenStates(t *testing.T) {
	scope := testScope("lookup-reopen-states")
	crypto := testJournalCrypto(t, scope)
	reject := JournalLookupVerifierFunc(func(*SOJournalLookup, *SOJournalVersionTuple) error {
		return errors.New("lookup authority rejected")
	})
	for _, fixture := range []struct {
		name  string
		state SOReceiptState
	}{
		{name: "pending", state: SOReceiptState_SO_RECEIPT_STATE_PENDING},
		{name: "no-record", state: SOReceiptState_SO_RECEIPT_STATE_NO_RECORD},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			key := testMutationKey(scope, "peer", fixture.name)
			lineage := testLineage(key, nil)
			version := JournalVersion(1, 1, 1, testDigest("config"))
			storage := newMemoryJournalStorage()
			pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
			if err != nil {
				t.Fatal(err)
			}
			intent := testIntent(t, crypto, key, lineage, version, 1, fixture.name)
			if err := pipeline.appendRecord(intent); err != nil {
				t.Fatal(err)
			}
			decoded := testDecodedIntent(t, crypto, intent, key, lineage, version, 1)
			envelope, err := NewJournalEnvelopeRecord(crypto, 2, decoded, []byte(fixture.name+"-envelope"), journalDefaultIdentity())
			if err != nil {
				t.Fatal(err)
			}
			if err := pipeline.appendRecord(envelope); err != nil {
				t.Fatal(err)
			}
			if err := pipeline.appendRecord(newJournalSentRecord(key, lineage, version)); err != nil {
				t.Fatal(err)
			}
			response := []byte(fixture.name + "-response")
			lookup := &SOJournalLookup{
				Key: key.CloneVT(), State: fixture.state, Response: response,
				ResponseDigest: testDigest(string(response)), ConfigChainDigest: testDigest("config"),
			}
			if err := pipeline.appendRecord(NewJournalReceiptLookupRecord(key, lineage, version, lookup)); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), reject); !errors.Is(err, ErrJournalLookupVerification) {
				t.Fatalf("reopen accepted rejected %s lookup: %v", fixture.name, err)
			}
		})
	}
}

func testReceiptVerifier() JournalReceiptVerifier {
	return JournalReceiptVerifierFunc(func(receipt *SOJournalReceipt, version *SOJournalVersionTuple) error {
		if receipt == nil || version == nil || len(receipt.GetTerminalReceipt()) == 0 || len(receipt.GetConfigChainDigest()) != sha256.Size {
			return errors.New("invalid opaque receipt verification input")
		}
		return nil
	})
}

func TestPhase1TypedNilAuthorities(t *testing.T) {
	scope := testScope("typed-nil")
	var authority JournalKeyAuthority = JournalKeyAuthorityFunc(nil)
	if _, err := NewJournalCrypto(scope, authority); !errors.Is(err, ErrJournalKeyAuthority) {
		t.Fatalf("typed-nil key authority accepted: %v", err)
	}
	crypto := testJournalCrypto(t, scope)
	var storage JournalStorage = (*memoryJournalStorage)(nil)
	var verifier JournalReceiptVerifier = JournalReceiptVerifierFunc(nil)
	var lookupVerifier JournalLookupVerifier = JournalLookupVerifierFunc(nil)
	if _, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier()); !errors.Is(err, ErrJournalStorageRequired) {
		t.Fatalf("typed-nil storage accepted: %v", err)
	}
	if _, err := OpenJournalPipelineWithCrypto(newMemoryJournalStorage(), crypto, verifier, testLookupVerifier()); !errors.Is(err, ErrJournalReceiptVerifierRequired) {
		t.Fatalf("typed-nil receipt verifier accepted: %v", err)
	}
	if _, err := OpenJournalPipelineWithCrypto(newMemoryJournalStorage(), crypto, testReceiptVerifier(), lookupVerifier); !errors.Is(err, ErrJournalLookupVerifierRequired) {
		t.Fatalf("typed-nil lookup verifier accepted: %v", err)
	}
}

func testLookupVerifier() JournalLookupVerifier {
	return JournalLookupVerifierFunc(func(lookup *SOJournalLookup, version *SOJournalVersionTuple) error {
		if lookup == nil || version == nil || len(lookup.GetResponse()) == 0 || len(lookup.GetResponseDigest()) != sha256.Size || !bytes.Equal(lookup.GetResponseDigest(), testDigest(string(lookup.GetResponse()))) || len(lookup.GetConfigChainDigest()) != sha256.Size {
			return errors.New("invalid opaque lookup verification input")
		}
		return nil
	})
}

func testJournalCrypto(t *testing.T, scope []byte) *JournalCrypto {
	return testJournalCryptoWithMaster(t, scope, testDigest("master"))
}

func testJournalCryptoWithMaster(t *testing.T, scope, master []byte) *JournalCrypto {
	t.Helper()
	crypto, err := NewJournalCrypto(scope, JournalKeyAuthorityFunc(func([]byte) ([]byte, error) {
		return master, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	return crypto
}

func testScope(value string) []byte {
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}

func testDigest(value string) []byte {
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}
func testMutationKey(scope []byte, peerID, localID string) *SOMutationKey {
	key, _ := NewSOMutationKey(scope, "shared-object", peerID, localID)
	return key
}

func testLineage(key, supersedes *SOMutationKey) *SOJournalLineage {
	return &SOJournalLineage{RootKey: key.CloneVT(), Supersedes: supersedes.CloneVT()}
}

func testIntent(t *testing.T, crypto *JournalCrypto, key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, sequence uint64, operation string) *SOJournalRecord {
	t.Helper()
	intent, err := NewJournalIntent(key, lineage, version, []byte(operation))
	if err != nil {
		t.Fatal(err)
	}
	record, err := NewJournalIntentRecord(crypto, sequence, intent, SOJournalReadiness_SO_JOURNAL_READINESS_READY, journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func testDecodedIntent(t *testing.T, crypto *JournalCrypto, record *SOJournalRecord, key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple, sequence uint64) *SOJournalIntent {
	t.Helper()
	plaintext, err := crypto.OpenWithIdentity(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, sequence, key, journalDefaultIdentity(), record.GetIntent())
	if err != nil {
		t.Fatal(err)
	}
	intent := new(SOJournalIntent)
	if err := intent.UnmarshalVT(plaintext); err != nil {
		t.Fatal(err)
	}
	intent.Key = key.CloneVT()
	intent.Lineage = lineage.CloneVT()
	intent.Version = version.CloneVT()
	return intent
}

func testIntentRecord(key *SOMutationKey, lineage *SOJournalLineage, version *SOJournalVersionTuple) *SOJournalRecord {
	return &SOJournalRecord{
		FormatVersion: JournalFormatVersion,
		Sequence:      1,
		Kind:          SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT,
		Key:           key.CloneVT(),
		Lineage:       lineage.CloneVT(),
		Version:       version.CloneVT(),
		Intent:        &SOJournalEncryptedPayload{Nonce: bytes.Repeat([]byte{1}, journalNonceSize), Ciphertext: bytes.Repeat([]byte{2}, 16)},
		Readiness:     SOJournalReadiness_SO_JOURNAL_READINESS_READY,
		AttemptState:  SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_INTENT_DURABLE,
	}
}

func testReceipt(key *SOMutationKey, envelopeDigest, terminal []byte, seqno uint64, rootDigest []byte) *SOJournalReceipt {
	return &SOJournalReceipt{
		Key:                     key.CloneVT(),
		EnvelopeDigest:          slicesClone(envelopeDigest),
		Outcome:                 SOJournalOutcome_SO_JOURNAL_OUTCOME_ACCEPTED,
		TerminalReceipt:         slicesClone(terminal),
		TerminalReceiptDigest:   testDigest(string(terminal)),
		AuthoritativeRootSeqno:  seqno,
		AuthoritativeRootDigest: slicesClone(rootDigest),
		ConfigChainDigest:       testDigest("config"),
	}
}

func slicesClone(value []byte) []byte {
	return append([]byte(nil), value...)
}
