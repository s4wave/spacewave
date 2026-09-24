package sobject

import (
	"reflect"
	"testing"
)

// TestJournalCheckpointAfterUnsyncedRetirement preserves acknowledged state across two failed retirements.
func TestJournalCheckpointAfterUnsyncedRetirement(t *testing.T) {
	scope := testScope("checkpoint after unsynced retirement")
	crypto := testJournalCrypto(t, scope)
	storage := &publicationFaultStorage{memoryJournalStorage: newMemoryJournalStorage()}
	pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}
	key := testMutationKey(scope, "peer", "operation")
	version := JournalVersion(1, 1, 1, testDigest("config"))
	if err := pipeline.appendRecord(testIntent(t, crypto, key, testLineage(key, nil), version, 1, "operation")); err != nil {
		t.Fatal(err)
	}
	acknowledged := pipeline.Snapshot()

	// Leave a visible empty segment whose durable bytes still contain the old frame.
	storage.fault, storage.armed = 11, true
	if err := pipeline.journal.checkpoint(); err == nil {
		t.Fatal("expected failure after visible retirement")
	}
	storage.armed = false
	recovered, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatal(err)
	}

	// Checkpoint immediately: an append would sync away the recovery defect.
	storage.fault, storage.armed = 9, true
	if err := recovered.journal.checkpoint(); err == nil {
		t.Fatal("expected retirement sync failure")
	}
	storage.armed = false
	storage.setSyncFailure(nil)
	recovered, err = OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
	if err != nil {
		t.Fatalf("acknowledged checkpoint did not recover: %v", err)
	}
	if !reflect.DeepEqual(acknowledged, recovered.Snapshot()) {
		t.Fatal("recovery lost acknowledged state")
	}
}
