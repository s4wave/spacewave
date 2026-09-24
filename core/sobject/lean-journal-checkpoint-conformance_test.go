package sobject

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"math"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/pkg/errors"
)

// TestLeanJournalCheckpointConformance checks preparation and every candidate publication boundary.
func TestLeanJournalCheckpointConformance(t *testing.T) {
	oracle := leanOracle(t)
	var cases []leanCase
	for seed := range uint64(20) {
		cases = append(cases, runLeanJournalCheckpointScenario(t, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanJournalCheckpoint searches generation selection, retained state and preparation failures.
func FuzzLeanJournalCheckpoint(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanJournalCheckpointScenario(t, seed))
	})
}

// checkpointFaultStorage captures attempted ciphertext before the caller scrubs it.
type checkpointFaultStorage struct {
	*activationFaultStorage
	sizeErr             bool
	attemptedGeneration uint64
	attempted           []byte
}

func (s *checkpointFaultStorage) Size() (int64, error) {
	if s.sizeErr {
		return 0, errors.New("injected checkpoint size failure")
	}
	return s.memoryJournalStorage.Size()
}

func (s *checkpointFaultStorage) ReadAt(data []byte, offset int64) (int, error) {
	if s.retiredReadErr {
		return 0, errors.New("injected checkpoint retired read failure")
	}
	return s.publicationFaultStorage.ReadAt(data, offset)
}

func (s *checkpointFaultStorage) WriteJournalCheckpointGeneration(generation uint64, data []byte) error {
	s.attemptedGeneration, s.attempted = generation, slices.Clone(data)
	return s.publicationFaultStorage.WriteJournalCheckpointGeneration(generation, data)
}

// runLeanJournalCheckpointScenario projects raw snapshot serialization without calling checkpoint admission.
func runLeanJournalCheckpointScenario(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	var cases []leanCase
	for variant := range 25 {
		faults := 1
		if variant == 0 {
			faults = 12
		}
		for fault := range faults {
			scope := testScope("lean checkpoint " + strconv.FormatUint(seed, 10))
			crypto := testJournalCrypto(t, scope)
			storage := &checkpointFaultStorage{activationFaultStorage: &activationFaultStorage{
				publicationFaultStorage: &publicationFaultStorage{memoryJournalStorage: newMemoryJournalStorage()},
			}}
			pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
			if err != nil {
				t.Fatal(err)
			}
			for index := range 2 {
				key := testMutationKey(scope, "peer", strconv.Itoa(index))
				version := JournalVersion(seed%17+1, seed%13+1, 1, testDigest("config"))
				if err := pipeline.appendRecord(testIntent(t, crypto, key, testLineage(key, nil), version, uint64(index+1), "operation")); err != nil {
					t.Fatal(err)
				}
				if index == 0 && seed%2 != 0 {
					if err := pipeline.journal.checkpoint(); err != nil {
						t.Fatal(err)
					}
				}
			}
			writer := pipeline.journal.writer
			target := pipeline.journal
			available, generationSupported, floorSupported, floorReadOK := true, true, true, true
			if len(storage.marker) == 0 && (variant == 9 || variant == 10 || variant == 11 || variant == 23 || variant == 24) {
				storage.marker, err = marshalJournalGenerationMarker(journalGenerationMarker{
					Identity: storage.identity, Generation: 1, NextSequence: writer.sequence,
					SnapshotLength: 1, SnapshotDigest: testDigest("snapshot"), RetiredDigest: testDigest("retired"),
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			switch variant {
			case 1:
				target, available = nil, false
			case 2:
				target, available = &journal{}, false
			case 3:
				writer.crypto = nil
			case 4:
				writer.storage, generationSupported = struct{ JournalStorage }{storage}, false
			case 5:
				writer.poisoned = ErrJournalWriterPoisoned
			case 6:
				writer.storage = activationWithoutFloor{JournalStorage: storage, JournalGenerationStore: storage}
				floorSupported = false
			case 7:
				floorReadOK = false
				storage.setGenerationFloorReadFailure(errors.New("injected checkpoint floor read failure"))
			case 8:
				storage.markerReadErr = true
			case 9:
				storage.marker[140] ^= 1
			case 10:
				storage.generationFloor = 3
			case 11:
				binary.BigEndian.PutUint64(storage.marker[40:48], storage.generationFloor+2)
				repairLeanJournalMarkerCRC(storage.marker)
			case 12:
				storage.marker, storage.generationFloor = nil, 1
			case 13:
				storage.sizeErr = true
			case 14:
				storage.retiredReadErr = true
			case 15:
				writer.generation = math.MaxUint64
			case 16:
				writer.sequence = 0
			case 17:
				writer.reducer = nil
			case 18:
				for _, attempt := range writer.reducer.attempts {
					attempt.State = 0
				}
			case 19:
				writer.crypto, err = NewJournalCrypto(scope, JournalKeyAuthorityFunc(func([]byte) ([]byte, error) {
					return nil, errors.New("injected checkpoint key failure")
				}))
				if err != nil {
					t.Fatal(err)
				}
			case 20:
				writer.identity = writer.identity[:31]
			case 21:
				writer.generation = 0
			case 22:
				writer.generation = 7
			case 23:
				storage.marker[8] ^= 1
				repairLeanJournalMarkerCRC(storage.marker)
			case 24:
				binary.BigEndian.PutUint32(storage.marker[4:8], 2)
				repairLeanJournalMarkerCRC(storage.marker)
			}
			before := projectLeanJournalPublication(t, writer, storage.memoryJournalStorage)
			retired := storage.bytes()
			retiredDigest := sha256.Sum256(retired)
			generation := max(writer.generation, storage.generationFloor, decodeLeanJournalMarker(storage.marker).Generation) + 1
			plaintext := leanCheckpointBytes(t, writer, generation)
			snapshotDigest := sha256.Sum256(plaintext)
			probe, sealErr := writer.crypto.SealCheckpointGeneration(writer.identity, generation, writer.sequence, plaintext)
			clear(probe)
			var marker any
			if len(storage.marker) != 0 {
				marker = projectLeanJournalMarkerObservation(storage.marker)
			}
			input := map[string]any{
				"journalAvailable": available, "crypto": writer.crypto != nil,
				"generationSupported": generationSupported, "floorSupported": floorSupported, "reducerAvailable": writer.reducer != nil,
				"identity": hex.EncodeToString(writer.identity), "floorReadOK": floorReadOK, "markerReadOK": !storage.markerReadErr,
				"marker": marker, "retiredReadOK": !storage.sizeErr && !storage.retiredReadErr,
				"encodedLength": uint64(len(plaintext)), "snapshotDigest": hex.EncodeToString(snapshotDigest[:]),
				"retiredDigest": hex.EncodeToString(retiredDigest[:]), "sealOK": sealErr == nil, "fault": int32(fault),
			}
			storage.attempted, storage.reads = nil, 0
			storage.fault, storage.armed = fault, true
			err = target.checkpoint()
			var prepared any
			if storage.attempted != nil {
				candidate := leanCheckpointBytes(t, writer, storage.attemptedGeneration)
				digest := sha256.Sum256(candidate)
				decoded, decodeErr := writer.crypto.OpenCheckpointGeneration(storage.attempted, writer.identity, storage.attemptedGeneration, writer.sequence, len(candidate), digest[:])
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				binding := journalGenerationMarker{
					Identity: writer.identity, Generation: storage.attemptedGeneration, NextSequence: writer.sequence,
					SnapshotLength: uint32(len(candidate)), SnapshotDigest: digest[:], //nolint:gosec // The bounded fixture is smaller than a frame.
					RetiredLength: uint64(len(retired)), RetiredDigest: retiredDigest[:],
				}
				prepared = map[string]any{"checkpoint": projectLeanJournalCheckpoint(t, decoded), "marker": projectLeanJournalMarker(binding)}
				clear(decoded)
			}
			request := map[string]any{"op": "checkpointJournalWriter", "before": before, "input": input}
			actual := map[string]any{"ok": err == nil, "result": projectLeanJournalPublication(t, writer, storage.memoryJournalStorage), "prepared": prepared}
			name := "checkpointJournalWriter seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant) + " fault " + strconv.Itoa(fault)
			cases = append(cases, leanCase{name: name, request: marshalLeanJournal(t, request), ok: err == nil, field: "checkpoint", value: marshalLeanJournal(t, actual)})
			if variant == 0 {
				storage.armed = false
				storage.setSyncFailure(nil)
				reopened, reopenErr := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
				if reopenErr != nil || !reflect.DeepEqual(writer.reducer.Snapshot(), reopened.Snapshot()) {
					t.Fatalf("%s: checkpoint recovery failed: %v", name, reopenErr)
				}
			}
			clear(plaintext)
		}
	}
	return append(cases, leanOutgoingMarkerCases(t)...)
}

// leanCheckpointBytes encodes raw snapshot fields without running retained-attempt or checkpoint admission.
func leanCheckpointBytes(t *testing.T, writer *journalWriter, generation uint64) []byte {
	t.Helper()
	checkpoint := &SOJournalCheckpoint{JournalIdentity: writer.identity, Generation: generation, NextSequence: writer.sequence}
	if writer.reducer != nil {
		for _, a := range writer.reducer.Snapshot() {
			checkpoint.Attempts = append(checkpoint.Attempts, &SOJournalCheckpointAttempt{
				Key: a.Key, Lineage: a.Lineage, Version: a.Version, State: a.State, Readiness: a.Readiness,
				Intent: a.Intent, Envelope: a.Envelope, EnvelopeDigest: a.EnvelopeDigest, Receipt: a.Receipt,
				Acknowledgement: a.Acknowledgement, Projection: a.Projection, Lookup: a.Lookup,
				IntentSequence: a.IntentSequence, EnvelopeSequence: a.EnvelopeSequence,
				CheckpointEligible: a.CheckpointEligible, SendAttempted: a.SendAttempted,
				ResendAuthorized: a.ResendAuthorized, LineageRecoveryBlocked: a.LineageRecoveryBlocked,
			})
		}
	}
	return mustMarshalVT(t, checkpoint)
}

// leanOutgoingMarkerCases checks the actual outgoing marker guard with invalid fixed-width metadata.
func leanOutgoingMarkerCases(t *testing.T) []leanCase {
	t.Helper()
	var cases []leanCase
	for variant := range 9 {
		marker := journalGenerationMarker{
			Identity: testDigest("identity"), Generation: 1, NextSequence: 3,
			SnapshotLength: 20, SnapshotDigest: testDigest("snapshot"), RetiredDigest: testDigest("retired"),
		}
		switch variant {
		case 1:
			marker.Identity = nil
		case 2:
			marker.Generation = 0
		case 3:
			marker.NextSequence = 0
		case 4:
			marker.SnapshotLength = 0
		case 5:
			marker.SnapshotDigest = nil
		case 6:
			marker.RetiredDigest = nil
		case 7:
			marker.Generation, marker.NextSequence, marker.RetiredLength = math.MaxUint64, math.MaxUint64, math.MaxUint64
		case 8:
			marker.Identity = append(marker.Identity, 0)
		}
		data, err := marshalJournalGenerationMarker(marker)
		request := map[string]any{"op": "validateOutgoingJournalMarker", "marker": projectLeanJournalMarker(marker)}
		cases = append(cases, leanCase{name: "validateOutgoingJournalMarker " + strconv.Itoa(variant), request: marshalLeanJournal(t, request), ok: err == nil})
		var observation any
		var crc uint32
		if err == nil {
			observation = projectLeanJournalMarkerObservation(data)
			crc = crc32.Checksum(data[:140], crc32.MakeTable(crc32.Castagnoli))
		}
		encoded := map[string]any{"op": "encodeJournalMarker", "marker": projectLeanJournalMarker(marker), "crc": crc}
		cases = append(cases, leanCase{name: "encodeJournalMarker " + strconv.Itoa(variant), request: marshalLeanJournal(t, encoded), ok: err == nil, field: "marker", value: marshalLeanJournal(t, observation)})
	}
	return cases
}
