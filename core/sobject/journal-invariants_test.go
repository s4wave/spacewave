package sobject

import (
	"reflect"
	"testing"
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
