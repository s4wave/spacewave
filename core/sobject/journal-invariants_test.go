package sobject

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"

	"github.com/pkg/errors"
)

// TestJournalResendCheckpointEquivalence preserves the exact reducer snapshot across compaction.
func TestJournalResendCheckpointEquivalence(t *testing.T) {
	scope := testScope("resend-checkpoint")
	crypto := testJournalCrypto(t, scope)
	pipeline := testPipeline(t, crypto)
	key := testMutationKey(scope, "peer", "resend")
	lineage := testLineage(key, nil)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	intent := testIntent(t, crypto, key, lineage, version, 1, "operation")
	if err := pipeline.appendRecord(intent); err != nil {
		t.Fatal(err)
	}
	decoded := testDecodedIntent(t, crypto, intent, key, lineage, version, 1)
	if err := pipeline.AppendEnvelope(decoded, []byte("signed envelope")); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.BeforeSend(key, 1, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	lookup := &SOJournalLookup{
		Key: key.CloneVT(), State: SOReceiptState_SO_RECEIPT_STATE_NO_RECORD,
		Response: []byte("no record"), ResponseDigest: testDigest("no record"),
		ConfigChainDigest: testDigest("config"),
	}
	if err := pipeline.AppendReceiptLookup(key, lookup); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.AppendResendAuthorization(key); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.BeforeSend(key, 1, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	// Compact the still-pending attempt, then compare all observable reducer evidence.
	before := pipeline.Snapshot()
	if err := pipeline.journal.checkpoint(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournalPipelineWithCrypto(pipeline.journal.writer.storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}
	after := reopened.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("checkpoint changed resend evidence: live lookup history=%v reopened=%v", before[0].LookupHistory, after[0].LookupHistory)
	}
}

// TestJournalFinalGenerationRecovers preserves the final published checkpoint at counter exhaustion.
func TestJournalFinalGenerationRecovers(t *testing.T) {
	crypto := testJournalCrypto(t, testScope("final-generation"))
	pipeline := testPipeline(t, crypto)
	key := testMutationKey(testScope("final-generation"), "peer", "intent")
	version := JournalVersion(1, 1, 1, testDigest("config"))
	if err := pipeline.appendRecord(testIntent(t, crypto, key, testLineage(key, nil), version, 1, "operation")); err != nil {
		t.Fatal(err)
	}

	// Seed the counter immediately before its last two publications.
	pipeline.journal.writer.generation = math.MaxUint64 - 2
	if err := pipeline.journal.checkpoint(); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.journal.checkpoint(); err != nil {
		t.Fatal(err)
	}
	storage := pipeline.journal.writer.storage
	reopened, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatalf("final checkpoint did not reopen: %v", err)
	}
	if !reflect.DeepEqual(pipeline.Snapshot(), reopened.Snapshot()) {
		t.Fatal("final checkpoint changed reducer state")
	}
	// Exhausted checkpoint generations still admit durable suffix mutations.
	nextKey := testMutationKey(testScope("final-generation"), "peer", "suffix")
	if err := reopened.appendRecord(testIntent(t, crypto, nextKey, testLineage(nextKey, nil), version, 2, "suffix")); err != nil {
		t.Fatal(err)
	}
	replayed, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil || !reflect.DeepEqual(reopened.Snapshot(), replayed.Snapshot()) {
		t.Fatalf("final-generation suffix did not recover: %v", err)
	}
	if err := reopened.journal.checkpoint(); err == nil {
		t.Fatal("exhausted checkpoint generation was reused")
	}
	floor, err := storage.(journalGenerationFloorStore).JournalGenerationFloor()
	if err != nil || floor != math.MaxUint64 {
		t.Fatalf("final generation floor changed: floor=%d err=%v", floor, err)
	}
}

// TestJournalPublicationFailureFencesWriter prevents acknowledged appends to a retired segment.
func TestJournalPublicationFailureFencesWriter(t *testing.T) {
	scope := testScope("publication-failure")
	crypto := testJournalCrypto(t, scope)
	pipeline := testPipeline(t, crypto)
	storage := pipeline.journal.writer.storage.(*memoryJournalStorage)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	firstKey := testMutationKey(scope, "peer", "first")
	if err := pipeline.appendRecord(testIntent(t, crypto, firstKey, testLineage(firstKey, nil), version, 1, "first")); err != nil {
		t.Fatal(err)
	}
	before := pipeline.Snapshot()
	storage.setGenerationFloorFailure(errors.New("injected floor publication failure"))
	if err := pipeline.journal.checkpoint(); err == nil {
		t.Fatal("failed generation floor update was accepted")
	}
	storage.setGenerationFloorFailure(nil)

	// A failed publication must force recovery before another append is acknowledged.
	secondKey := testMutationKey(scope, "peer", "second")
	second := testIntent(t, crypto, secondKey, testLineage(secondKey, nil), version, 2, "second")
	appendErr := pipeline.appendRecord(second)
	reopened, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if appendErr == nil {
		t.Fatalf("failed publication acknowledged another append; recovery error: %v", err)
	}
	if !errors.Is(appendErr, ErrJournalWriterPoisoned) {
		t.Fatalf("incomplete publication append error: %v", appendErr)
	}
	if err != nil || !reflect.DeepEqual(before, reopened.Snapshot()) {
		t.Fatalf("publication failure lost acknowledged prefix: %v", err)
	}
	if err := reopened.appendRecord(second); err != nil {
		t.Fatalf("recovered publication did not resume appends: %v", err)
	}
}

// TestJournalTornPayloadTrailerRecovers preserves acknowledged records when payload bytes resemble a trailer.
func TestJournalTornPayloadTrailerRecovers(t *testing.T) {
	scope := testScope("torn-payload-trailer")
	crypto := testJournalCrypto(t, scope)
	pipeline := testPipeline(t, crypto)
	storage := pipeline.journal.writer.storage.(*memoryJournalStorage)
	version := JournalVersion(1, 1, 1, testDigest("config"))
	first := testMutationKey(scope, "peer", "first")
	if err := pipeline.appendRecord(testIntent(t, crypto, first, testLineage(first, nil), version, 1, "first")); err != nil {
		t.Fatal(err)
	}
	acknowledged := pipeline.Snapshot()
	second := testMutationKey(scope, "peer", "second")
	record := testIntent(t, crypto, second, testLineage(second, nil), version, 2, "second")
	cut := addJournalPayloadTrailer(t, record)
	storage.setWriteFailure(cut, errors.New("injected torn payload"))
	if err := pipeline.appendRecord(record); err == nil {
		t.Fatal("torn append was acknowledged")
	}
	storage.setWriteFailure(0, nil)
	recovered, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatalf("payload trailer prevented acknowledged-prefix recovery: %v", err)
	}
	if !reflect.DeepEqual(acknowledged, recovered.Snapshot()) {
		t.Fatal("torn payload changed acknowledged prefix")
	}
	if err := recovered.appendRecord(record); err != nil {
		t.Fatalf("complete record containing trailer bytes did not append: %v", err)
	}
}

// addJournalPayloadTrailer retains a valid unknown protobuf field with trailer-like payload bytes.
// It returns the partial frame length ending immediately after those bytes.
func addJournalPayloadTrailer(t *testing.T, record *SOJournalRecord) int {
	t.Helper()
	payload := mustMarshalVT(t, record)
	unknown := make([]byte, 16)
	copy(unknown, journalTrailerMagic[:])
	binary.BigEndian.PutUint32(unknown[4:8], uint32(len(payload)+3))
	encoded := append(append(payload, 0xc2, 0x3e, byte(len(unknown))), unknown...)
	if err := record.UnmarshalVT(encoded); err != nil {
		t.Fatal(err)
	}
	return journalHeaderSize + len(escapeJournalPayload(encoded[:len(payload)+3+8]))
}

// TestJournalEscapedPayloadBounds preserves the full raw payload limit after worst-case escaping.
func TestJournalEscapedPayloadBounds(t *testing.T) {
	payload := make([]byte, journalMaxPayload)
	frame, err := marshalJournalFrame(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) != journalHeaderSize+journalMaxFramePayload+journalTrailerSize {
		t.Fatalf("worst-case frame length = %d", len(frame))
	}
	decoded, err := unescapeJournalPayload(frame[journalHeaderSize : len(frame)-journalTrailerSize])
	if err != nil || len(decoded) != journalMaxPayload {
		t.Fatalf("maximum raw payload did not roundtrip: %v", err)
	}
	if _, err := marshalJournalFrame(SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, 1, append(payload, 0)); err == nil {
		t.Fatal("oversized raw payload was accepted")
	}
	if _, err := unescapeJournalPayload(append(frame[journalHeaderSize:len(frame)-journalTrailerSize], 1)); err == nil {
		t.Fatal("oversized decoded payload was accepted")
	}
}
