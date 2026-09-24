package sobject

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/pkg/errors"
)

// TestLeanJournalPipelineConformance checks public prerequisites before recovery and activation effects.
func TestLeanJournalPipelineConformance(t *testing.T) {
	oracle := leanOracle(t)
	var cases []leanCase
	for seed := range uint64(20) {
		cases = append(cases, runLeanJournalPipelineScenario(t, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanJournalPipeline searches public capability, retained authority and recovery boundaries.
func FuzzLeanJournalPipeline(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanJournalPipelineScenario(t, seed))
	})
}

// runLeanJournalPipelineScenario starts from real terminal evidence and projects primitive inputs independently.
func runLeanJournalPipelineScenario(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	scope := testScope("lean public pipeline " + strconv.FormatUint(seed, 10))
	crypto := testJournalCrypto(t, scope)
	version := JournalVersion(1, seed%17+1, 1, testDigest("config"))
	var cases []leanCase
	for mode := range 4 {
		for variant := range 16 {
			storage := &activationFaultStorage{publicationFaultStorage: &publicationFaultStorage{memoryJournalStorage: newMemoryJournalStorage()}}
			pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
			if err != nil {
				t.Fatal(err)
			}
			key := testMutationKey(scope, "peer", "terminal")
			lineage := testLineage(key, nil)
			intent := testIntent(t, crypto, key, lineage, version, 1, "operation")
			decoded := testDecodedIntent(t, crypto, intent, key, lineage, version, 1)
			envelope, err := NewJournalEnvelopeRecord(crypto, 2, decoded, []byte("signed envelope"), storage.identity)
			if err != nil {
				t.Fatal(err)
			}
			receipt := testReceipt(key, envelope.EnvelopeDigest, []byte("terminal"), 1, testDigest("root"))
			lookup := &SOJournalLookup{
				Key: key.CloneVT(), State: SOReceiptState_SO_RECEIPT_STATE_ACCEPTED, Receipt: receipt,
				Response: []byte("lookup"), ResponseDigest: testDigest("lookup"), ConfigChainDigest: version.ConfigChainDigest,
			}
			for _, record := range []*SOJournalRecord{
				intent, envelope, newJournalSentRecord(key, lineage, version),
				NewJournalReceiptLookupRecord(key, lineage, version, lookup),
			} {
				if err := pipeline.appendRecord(record); err != nil {
					t.Fatal(err)
				}
			}
			if mode >= 2 {
				storage.fault, storage.armed = 4, mode == 3
				if err := pipeline.journal.checkpoint(); (err != nil) != (mode == 3) {
					t.Fatalf("prepare mode %d: %v", mode, err)
				}
				storage.armed = false
			}
			if mode == 1 {
				key := testMutationKey(scope, "peer", "torn")
				record := testIntent(t, crypto, key, testLineage(key, nil), version, 5, "torn")
				frame, err := marshalJournalFrame(record.Kind, 5, mustMarshalVT(t, record))
				if err != nil {
					t.Fatal(err)
				}
				cut := 1 + int(seed%uint64(len(frame)-1))
				storage.data = append(storage.data, frame[:cut]...)
				if err := storage.Sync(); err != nil {
					t.Fatal(err)
				}
			}
			activeCrypto := crypto
			var target JournalStorage = storage
			storageAvailable, receiptAvailable, lookupAvailable := true, true, true
			receiptOK, lookupOK := variant != 8, variant != 9
			var verifier JournalReceiptVerifier = JournalReceiptVerifierFunc(func(receipt *SOJournalReceipt, version *SOJournalVersionTuple) error {
				if !receiptOK {
					return errors.New("injected public receipt authority rejection")
				}
				return testReceiptVerifier().VerifyJournalReceipt(receipt, version)
			})
			var lookupVerifier JournalLookupVerifier = JournalLookupVerifierFunc(func(lookup *SOJournalLookup, version *SOJournalVersionTuple) error {
				if !lookupOK {
					return errors.New("injected public lookup authority rejection")
				}
				return testLookupVerifier().VerifyJournalLookup(lookup, version)
			})
			openFault, activationFault := int32(0), int32(0)
			switch variant {
			case 1:
				target, storageAvailable = nil, false
			case 2:
				target, storageAvailable = (*activationFaultStorage)(nil), false
			case 3:
				activeCrypto = nil
			case 4:
				verifier, receiptAvailable = nil, false
			case 5:
				verifier, receiptAvailable = JournalReceiptVerifierFunc(nil), false
			case 6:
				lookupVerifier, lookupAvailable = nil, false
			case 7:
				lookupVerifier, lookupAvailable = JournalLookupVerifierFunc(nil), false
			case 10:
				activeCrypto = testJournalCrypto(t, testScope("another account"))
			case 11, 12, 13, 14, 15:
				storage.armed = true
				storage.fault = []int{4, 5, 8, 11, 9}[variant-11]
				activationFault = int32(variant - 10) //nolint:gosec // variant is bounded by the loop.
				if variant >= 13 {
					openFault = int32(variant - 12) //nolint:gosec // variant is bounded by the loop.
				}
			}
			data := storage.bytes()
			retiredDigest := sha256.Sum256(data)
			var marker, checkpoint any
			if len(storage.marker) != 0 {
				marker = projectLeanJournalMarkerObservation(storage.marker)
				fields := decodeLeanJournalMarker(storage.marker)
				plaintext, openErr := activeCrypto.OpenCheckpointGeneration(storage.generations[fields.Generation], storage.identity, fields.Generation, fields.NextSequence, int(fields.SnapshotLength), fields.SnapshotDigest)
				if openErr == nil {
					checkpoint = projectLeanJournalCheckpoint(t, plaintext)
					clear(plaintext)
				}
			}
			input := map[string]any{
				"bytes":            map[string]any{"data": leanJournalBytes(data), "durable": leanJournalBytes(storage.durable)},
				"storageAvailable": storageAvailable, "generationSupported": true, "floorSupported": true,
				"identity": hex.EncodeToString(storage.identity), "crypto": activeCrypto != nil,
				"markerReadOK": true, "marker": marker, "floorReadOK": true, "floor": storage.generationFloor,
				"checkpoint": checkpoint, "scanSizeOK": true, "frames": projectLeanJournalFramePrimitives(projectLeanJournalFrames(t, data)),
				"retiredReadOK": true, "retiredDigest": hex.EncodeToString(retiredDigest[:]), "tailSizeOK": true, "fault": openFault,
			}
			activation := map[string]any{
				"generationSupported": true, "floorSupported": true, "floorReadOK": true, "floor": storage.generationFloor,
				"marker": marker, "retiredReadOK": true, "retiredDigest": hex.EncodeToString(retiredDigest[:]), "fault": activationFault,
			}
			auth := projectLeanRetainedAuthentication(t, decodeLeanJournalRecords(data), pipeline.Snapshot(), activeCrypto, storage.identity)
			request := map[string]any{
				"op": "openJournalPipeline", "input": input, "activation": activation, "auth": auth,
				"receiptAvailable": receiptAvailable, "lookupAvailable": lookupAvailable, "receiptOK": receiptOK, "lookupOK": lookupOK,
			}
			reopened, openErr := OpenJournalPipelineWithCrypto(target, activeCrypto, verifier, lookupVerifier)
			var writer any
			if openErr == nil {
				writer = projectLeanJournalWriter(t, reopened.journal.writer, storage.memoryJournalStorage)
			}
			actual := map[string]any{
				"writer": writer, "bytes": map[string]any{"data": leanJournalBytes(storage.bytes()), "durable": leanJournalBytes(storage.durable)},
				"floor": storage.generationFloor,
			}
			name := "openJournalPipeline seed " + strconv.FormatUint(seed, 10) + " mode " + strconv.Itoa(mode) + " variant " + strconv.Itoa(variant)
			cases = append(cases, leanCase{name: name, request: marshalLeanJournal(t, request), ok: openErr == nil, field: "pipeline", value: marshalLeanJournal(t, actual)})
		}
	}
	return cases
}
