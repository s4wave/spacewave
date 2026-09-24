package sobject

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/pkg/errors"
)

// TestLeanJournalTraceConformance recovers every publication cut with both visible and crash-restored bytes.
func TestLeanJournalTraceConformance(t *testing.T) {
	oracle := leanOracle(t)
	var cases []leanCase
	for seed := range uint64(12) {
		cases = append(cases, runLeanJournalTraceScenario(t, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanJournalTrace searches composed checkpoint publication and public recovery.
func FuzzLeanJournalTrace(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanJournalTraceScenario(t, seed))
	})
}

// traceFaultStorage injects reread failures even when the retired segment is empty.
type traceFaultStorage struct {
	*checkpointFaultStorage
	sizes int
}

func (s *traceFaultStorage) Size() (int64, error) {
	size, err := s.memoryJournalStorage.Size()
	if s.armed {
		s.sizes++
		if s.sizes == 2 {
			switch s.fault {
			case 6:
				return 0, errors.New("injected retired size failure")
			case 7:
				return size + 1, nil
			}
		}
	}
	return size, err
}

func (s *traceFaultStorage) ReadAt(data []byte, offset int64) (int, error) {
	if s.armed && s.sizes == 2 && s.fault == 7 {
		clear(data)
		return len(data), nil
	}
	return s.memoryJournalStorage.ReadAt(data, offset)
}

// runLeanJournalTraceScenario supplies primitive decoding while Lean derives the post-publication recovery input.
func runLeanJournalTraceScenario(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	scope := testScope("lean checkpoint trace " + strconv.FormatUint(seed, 10))
	crypto := testJournalCrypto(t, scope)
	version := JournalVersion(seed%17+1, seed%13+1, 1, testDigest("config"))
	var cases []leanCase
	for mode := range 3 {
		for fault := range 12 {
			for _, crash := range []bool{false, true} {
				storage := &traceFaultStorage{checkpointFaultStorage: &checkpointFaultStorage{
					activationFaultStorage: &activationFaultStorage{publicationFaultStorage: &publicationFaultStorage{
						memoryJournalStorage: newMemoryJournalStorage(),
					}},
				}}
				pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
				if err != nil {
					t.Fatal(err)
				}
				for index := range 2 {
					key := testMutationKey(scope, "peer", strconv.Itoa(index))
					lineage := testLineage(key, nil)
					sequence := pipeline.journal.writer.sequence
					record := testIntent(t, crypto, key, lineage, version, sequence, "operation")
					if err := pipeline.appendRecord(record); err != nil {
						t.Fatal(err)
					}
					if index == 0 && seed%3 != 0 {
						decoded := testDecodedIntent(t, crypto, record, key, lineage, version, sequence)
						envelope, err := NewJournalEnvelopeRecord(crypto, pipeline.journal.writer.sequence, decoded, []byte("envelope"), storage.identity)
						if err != nil {
							t.Fatal(err)
						}
						for _, stage := range []*SOJournalRecord{envelope, newJournalSentRecord(key, lineage, version)} {
							if err := pipeline.appendRecord(stage); err != nil {
								t.Fatal(err)
							}
						}
						if seed%3 == 2 {
							lookup := &SOJournalLookup{
								Key: key.CloneVT(), State: SOReceiptState_SO_RECEIPT_STATE_ACCEPTED,
								Receipt:  testReceipt(key, envelope.EnvelopeDigest, []byte("terminal"), 1, testDigest("root")),
								Response: []byte("lookup"), ResponseDigest: testDigest("lookup"), ConfigChainDigest: version.ConfigChainDigest,
							}
							if err := pipeline.appendRecord(NewJournalReceiptLookupRecord(key, lineage, version, lookup)); err != nil {
								t.Fatal(err)
							}
						}
					}
					if (mode == 1 && index == 0) || (mode == 2 && index == 1) {
						if err := pipeline.journal.checkpoint(); err != nil {
							t.Fatal(err)
						}
					}
				}
				writer := pipeline.journal.writer
				acknowledged := pipeline.Snapshot()
				before := projectLeanJournalPublication(t, writer, storage.memoryJournalStorage)
				retired := storage.bytes()
				retiredDigest := sha256.Sum256(retired)
				generation := writer.generation + 1
				plaintext := leanCheckpointBytes(t, writer, generation)
				snapshotDigest := sha256.Sum256(plaintext)
				binding := journalGenerationMarker{
					Identity: writer.identity, Generation: generation, NextSequence: writer.sequence,
					SnapshotLength: uint32(len(plaintext)), SnapshotDigest: snapshotDigest[:], //nolint:gosec // The fixture contains two attempts.
					RetiredLength: uint64(len(retired)), RetiredDigest: retiredDigest[:],
				}
				markerBytes, err := marshalJournalGenerationMarker(binding)
				if err != nil {
					t.Fatal(err)
				}
				var oldMarker any
				if len(storage.marker) != 0 {
					oldMarker = projectLeanJournalMarkerObservation(storage.marker)
				}
				preparation := map[string]any{
					"journalAvailable": true, "crypto": true, "generationSupported": true, "floorSupported": true, "reducerAvailable": true,
					"identity": hex.EncodeToString(writer.identity), "floorReadOK": true, "markerReadOK": true, "marker": oldMarker,
					"retiredReadOK": true, "encodedLength": uint64(len(plaintext)), "snapshotDigest": hex.EncodeToString(snapshotDigest[:]),
					"retiredDigest": hex.EncodeToString(retiredDigest[:]), "sealOK": true, "fault": int32(fault),
				}
				storage.sizes, storage.attempted = 0, nil
				storage.fault, storage.armed = fault, true
				checkpointErr := pipeline.journal.checkpoint()
				if storage.attempted == nil || storage.attemptedGeneration != generation {
					t.Fatal("valid checkpoint did not attempt its candidate")
				}
				decoded, err := crypto.OpenCheckpointGeneration(storage.attempted, writer.identity, generation, writer.sequence, len(plaintext), snapshotDigest[:])
				if err != nil {
					t.Fatal(err)
				}
				checkpointResult := map[string]any{
					"ok": checkpointErr == nil, "result": projectLeanJournalPublication(t, writer, storage.memoryJournalStorage),
					"prepared": map[string]any{"checkpoint": projectLeanJournalCheckpoint(t, decoded), "marker": projectLeanJournalMarker(binding)},
				}
				clear(decoded)
				clear(plaintext)
				storage.armed = false
				storage.setSyncFailure(nil)
				if crash {
					storage.data = slices.Clone(storage.durable)
				}
				data := storage.bytes()
				observedDigest := sha256.Sum256(data)
				var checkpoint any
				if len(storage.marker) != 0 {
					selected := decodeLeanJournalMarker(storage.marker)
					decoded, err := crypto.OpenCheckpointGeneration(storage.generations[selected.Generation], storage.identity,
						selected.Generation, selected.NextSequence, int(selected.SnapshotLength), selected.SnapshotDigest)
					if err != nil {
						t.Fatal(err)
					}
					checkpoint = projectLeanJournalCheckpoint(t, decoded)
					clear(decoded)
				}
				input := map[string]any{
					"bytes": map[string]any{"data": []any{}, "durable": []any{}}, "marker": nil, "floor": uint64(0),
					"storageAvailable": true, "generationSupported": true, "floorSupported": true, "crypto": true,
					"identity": hex.EncodeToString(storage.identity), "markerReadOK": true, "floorReadOK": true,
					"checkpoint": checkpoint, "scanSizeOK": true, "frames": projectLeanJournalFramePrimitives(projectLeanJournalFrames(t, data)),
					"retiredReadOK": true, "retiredDigest": hex.EncodeToString(observedDigest[:]), "tailSizeOK": true, "fault": int32(0),
				}
				activation := map[string]any{
					"generationSupported": true, "floorSupported": true, "floorReadOK": true, "floor": uint64(0), "marker": nil,
					"retiredReadOK": true, "retiredDigest": hex.EncodeToString(observedDigest[:]), "fault": int32(0),
				}
				auth := projectLeanRetainedAuthentication(t, decodeLeanJournalRecords(data), acknowledged, crypto, storage.identity)
				request := map[string]any{
					"op": "checkpointAndOpenJournal", "before": before, "preparation": preparation, "input": input, "activation": activation,
					"crash": crash, "markerCRC": uint64(binary.BigEndian.Uint32(markerBytes[len(markerBytes)-4:])), "auth": auth,
					"receiptAvailable": true, "lookupAvailable": true, "receiptOK": true, "lookupOK": true,
				}
				reopened, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
				name := "checkpointAndOpenJournal seed " + strconv.FormatUint(seed, 10) + " mode " + strconv.Itoa(mode) + " fault " + strconv.Itoa(fault) + " crash " + strconv.FormatBool(crash)
				if err != nil {
					t.Fatalf("%s: recovery failed: %v", name, err)
				}
				if !reflect.DeepEqual(acknowledged, reopened.Snapshot()) {
					t.Fatalf("%s: recovery changed acknowledged state", name)
				}
				actual := map[string]any{
					"checkpoint": checkpointResult,
					"pipeline": map[string]any{
						"writer": projectLeanJournalWriter(t, reopened.journal.writer, storage.memoryJournalStorage),
						"bytes":  map[string]any{"data": leanJournalBytes(storage.bytes()), "durable": leanJournalBytes(storage.durable)},
						"floor":  storage.generationFloor,
					},
				}
				cases = append(cases, leanCase{name: name, request: marshalLeanJournal(t, request), ok: true, field: "trace", value: marshalLeanJournal(t, actual)})
			}
		}
	}
	return cases
}
