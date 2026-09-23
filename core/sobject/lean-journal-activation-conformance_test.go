package sobject

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"reflect"
	"strconv"
	"testing"

	"github.com/pkg/errors"
)

// TestLeanJournalActivationConformance checks marker admission and each pending activation boundary.
func TestLeanJournalActivationConformance(t *testing.T) {
	oracle := leanOracle(t)
	var cases []leanCase
	for seed := range uint64(20) {
		cases = append(cases, runLeanJournalActivationScenario(t, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanJournalActivation searches captured markers, retained bytes and activation failures.
func FuzzLeanJournalActivation(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanJournalActivationScenario(t, seed))
	})
}

// activationFaultStorage adds read failures to the shared publication fault fixture.
type activationFaultStorage struct {
	*publicationFaultStorage
	markerReadErr  bool
	retiredReadErr bool
}

func (s *activationFaultStorage) ReadJournalGeneration() ([]byte, error) {
	if s.markerReadErr {
		return nil, errors.New("injected marker read failure")
	}
	return s.memoryJournalStorage.ReadJournalGeneration()
}

func (s *activationFaultStorage) ReadAt(data []byte, offset int64) (int, error) {
	if s.retiredReadErr {
		return 0, errors.New("injected retired read failure")
	}
	return s.memoryJournalStorage.ReadAt(data, offset)
}

// activationWithoutFloor exposes generation storage without the required floor interface.
type activationWithoutFloor struct {
	JournalStorage
	JournalGenerationStore
}

// runLeanJournalActivationScenario starts from a real interrupted encrypted checkpoint publication.
func runLeanJournalActivationScenario(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	scope := testScope("lean activation " + strconv.FormatUint(seed, 10))
	crypto := testJournalCrypto(t, scope)
	version := JournalVersion(seed%17+1, seed%13+1, 1, testDigest("config"))
	var cases []leanCase
	for variant := range 28 {
		storage := &activationFaultStorage{publicationFaultStorage: &publicationFaultStorage{
			memoryJournalStorage: newMemoryJournalStorage(), fault: 4,
		}}
		pipeline, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
		if err != nil {
			t.Fatal(err)
		}
		for index := range 2 {
			key := testMutationKey(scope, "peer", strconv.Itoa(index))
			if err := pipeline.appendRecord(testIntent(t, crypto, key, testLineage(key, nil), version, uint64(index+1), "operation")); err != nil {
				t.Fatal(err)
			}
			if index == 0 && seed%2 != 0 {
				if err := pipeline.journal.checkpoint(); err != nil {
					t.Fatal(err)
				}
			}
		}
		storage.armed = true
		if err := pipeline.journal.checkpoint(); err == nil {
			t.Fatal("checkpoint did not stop at the injected floor failure")
		}
		storage.armed = false
		writer, _, err := openJournalWriter(storage, crypto)
		if err != nil || writer.pending == nil {
			t.Fatalf("open pending writer: %v", err)
		}
		if variant == 0 {
			cases = append(cases, leanJournalMarkerCases(t, storage.marker, storage.identity, seed)...)
		}
		fault := 0
		generationSupported, floorSupported, floorReadOK := true, true, true
		switch variant {
		case 1, 2, 3, 4, 5:
			fault = variant
			storage.fault = []int{0, 4, 5, 8, 11, 9}[variant]
			storage.armed = true
		case 6:
			writer.poisoned = ErrJournalWriterPoisoned
		case 7:
			writer.pending = nil
		case 8:
			storage.generationFloor++
		case 9, 10, 11, 12, 13, 14:
			index := []int{40, 48, 56, 60, 92, 100}[variant-9]
			storage.marker[index] ^= 1
			repairLeanJournalMarkerCRC(storage.marker)
		case 15:
			storage.marker[140] ^= 1
		case 16:
			floorReadOK = false
			storage.setGenerationFloorReadFailure(errors.New("injected floor read failure"))
		case 17:
			storage.data[len(storage.data)-1] ^= 1
		case 18:
			storage.marker = nil
		case 19:
			writer.identity = testDigest("another identity")
		case 20:
			generationSupported = false
			writer.storage = struct{ JournalStorage }{storage}
		case 21:
			floorSupported = false
			writer.storage = activationWithoutFloor{JournalStorage: storage, JournalGenerationStore: storage}
		case 22:
			storage.retiredReadErr = true
		case 23:
			storage.markerReadErr = true
		case 24:
			generation := storage.generationFloor + 3
			binary.BigEndian.PutUint64(storage.marker[40:48], generation)
			writer.pending.marker.Generation = generation
			repairLeanJournalMarkerCRC(storage.marker)
		case 25:
			storage.data = storage.data[:len(storage.data)-1]
		case 26:
			storage.marker[133] = byte(seed%255 + 1)
			repairLeanJournalMarkerCRC(storage.marker)
		case 27:
			storage.generationFloor = writer.pending.marker.Generation
			writer.pending.floor = storage.generationFloor
		}
		before := projectLeanJournalWriter(t, writer, storage.memoryJournalStorage)
		retiredDigest := sha256.Sum256(storage.bytes())
		var marker any
		if !storage.markerReadErr {
			marker = projectLeanJournalMarkerObservation(storage.marker)
		}
		input := map[string]any{
			"generationSupported": generationSupported, "floorSupported": floorSupported,
			"floor": storage.generationFloor, "floorReadOK": floorReadOK, "marker": marker,
			"retiredReadOK": !storage.retiredReadErr, "retiredDigest": hex.EncodeToString(retiredDigest[:]), "fault": int32(fault),
		}
		snapshot := writer.reducer.Snapshot()
		err = writer.activatePending()
		actual := map[string]any{"ok": err == nil, "result": projectLeanJournalWriter(t, writer, storage.memoryJournalStorage), "floor": storage.generationFloor}
		request := map[string]any{"op": "activateJournalWriter", "before": before, "input": input}
		name := "activateJournalWriter seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant)
		cases = append(cases, leanCase{name: name, request: marshalLeanJournal(t, request), ok: err == nil, field: "activation", value: marshalLeanJournal(t, actual)})
		if err == nil && variant != 7 {
			recovered, openErr := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
			if openErr != nil || !reflect.DeepEqual(snapshot, recovered.Snapshot()) {
				t.Fatalf("%s: activated checkpoint did not recover: %v", name, openErr)
			}
		}
	}
	cases = append(cases, leanJournalPipelineActivationCases(t, seed)...)
	return cases
}

// leanJournalPipelineActivationCases checks external authority before any pending retirement effect.
func leanJournalPipelineActivationCases(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	scope := testScope("lean pipeline activation " + strconv.FormatUint(seed, 10))
	crypto := testJournalCrypto(t, scope)
	version := JournalVersion(1, seed%17+1, 1, testDigest("config"))
	var cases []leanCase
	for variant := range 8 {
		storage := &publicationFaultStorage{memoryJournalStorage: newMemoryJournalStorage(), fault: 4}
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
		storage.armed = true
		if err := pipeline.journal.checkpoint(); err == nil {
			t.Fatal("checkpoint did not stop at floor publication")
		}
		storage.armed = false
		writer, _, err := openJournalWriter(storage, crypto)
		if err != nil || writer.pending == nil {
			t.Fatalf("open retained terminal checkpoint: %v", err)
		}
		activeCrypto := crypto
		receiptOK, lookupOK := variant != 1 && variant != 7, variant != 2
		var verifier JournalReceiptVerifier = JournalReceiptVerifierFunc(func(receipt *SOJournalReceipt, version *SOJournalVersionTuple) error {
			if !receiptOK {
				return errors.New("injected receipt authority rejection")
			}
			return testReceiptVerifier().VerifyJournalReceipt(receipt, version)
		})
		var lookupVerifier JournalLookupVerifier = JournalLookupVerifierFunc(func(lookup *SOJournalLookup, version *SOJournalVersionTuple) error {
			if !lookupOK {
				return errors.New("injected lookup authority rejection")
			}
			return testLookupVerifier().VerifyJournalLookup(lookup, version)
		})
		switch variant {
		case 3:
			verifier = nil
		case 4:
			lookupVerifier = nil
		case 5:
			activeCrypto = nil
		case 6:
			activeCrypto = testJournalCrypto(t, testScope("another account"))
		case 7:
			storage.armed = true
			storage.fault = 5
		}
		before := projectLeanJournalWriter(t, writer, storage.memoryJournalStorage)
		retiredDigest := sha256.Sum256(storage.bytes())
		input := map[string]any{
			"generationSupported": true, "floorSupported": true, "floor": storage.generationFloor, "floorReadOK": true,
			"marker": projectLeanJournalMarkerObservation(storage.marker), "retiredReadOK": true,
			"retiredDigest": hex.EncodeToString(retiredDigest[:]), "fault": int32(0),
		}
		if variant == 7 {
			input["fault"] = int32(2)
		}
		request := map[string]any{
			"op": "finishJournalPipeline", "before": before, "input": input,
			"crypto": activeCrypto != nil, "receiptAvailable": verifier != nil, "lookupAvailable": lookupVerifier != nil,
			"receiptOK": receiptOK, "lookupOK": lookupOK, "auth": projectLeanPipelineAuthentication(t, writer, activeCrypto),
		}
		reopened, err := OpenJournalPipelineWithCrypto(storage, activeCrypto, verifier, lookupVerifier)
		if err == nil {
			writer = reopened.journal.writer
		}
		actual := map[string]any{"ok": err == nil, "result": projectLeanJournalWriter(t, writer, storage.memoryJournalStorage), "floor": storage.generationFloor}
		name := "finishJournalPipeline seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant)
		cases = append(cases, leanCase{name: name, request: marshalLeanJournal(t, request), ok: err == nil, field: "activation", value: marshalLeanJournal(t, actual)})
	}
	return cases
}

// projectLeanPipelineAuthentication supplies primitive outputs for each exact retained-stage input.
func projectLeanPipelineAuthentication(t *testing.T, writer *journalWriter, crypto *JournalCrypto) []any {
	t.Helper()
	return projectLeanRetainedAuthentication(t, writer.records, writer.reducer.Snapshot(), crypto, writer.identity)
}

// projectLeanRetainedAuthentication projects primitive outputs for decoded records and retained stages.
func projectLeanRetainedAuthentication(t *testing.T, source []*SOJournalRecord, attempts []*JournalAttemptSnapshot, crypto *JournalCrypto, identity []byte) []any {
	t.Helper()
	records := append([]*SOJournalRecord(nil), source...)
	for _, attempt := range attempts {
		records = append(records, &SOJournalRecord{
			Kind: SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT, Sequence: attempt.IntentSequence,
			Key: attempt.Key, Lineage: attempt.Lineage, Version: attempt.Version, Intent: attempt.Intent,
		})
		if attempt.Envelope != nil {
			records = append(records, &SOJournalRecord{
				Kind: SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE, Sequence: attempt.EnvelopeSequence,
				Key: attempt.Key, Envelope: attempt.Envelope, EnvelopeDigest: attempt.EnvelopeDigest,
			})
		}
	}
	result := make([]any, len(records))
	for index, record := range records {
		result[index] = map[string]any{"record": projectLeanJournalRecord(t, record), "auth": projectLeanJournalAuthentication(t, record, crypto, identity)}
	}
	return result
}

// leanJournalMarkerCases checks every truncated length and corrupted byte plus repaired structural mutations.
func leanJournalMarkerCases(t *testing.T, original, identity []byte, seed uint64) []leanCase {
	t.Helper()
	var inputs [][]byte
	for cut := 0; cut <= len(original); cut++ {
		inputs = append(inputs, append([]byte(nil), original[:cut]...))
	}
	for index := range len(original) {
		data := append([]byte(nil), original...)
		data[index] ^= byte(seed%255 + 1)
		inputs = append(inputs, data)
	}
	for _, field := range []struct{ offset, width int }{{4, 4}, {40, 8}, {48, 8}, {56, 4}} {
		data := append([]byte(nil), original...)
		clear(data[field.offset : field.offset+field.width])
		repairLeanJournalMarkerCRC(data)
		inputs = append(inputs, data)
	}
	format := append([]byte(nil), original...)
	binary.BigEndian.PutUint32(format[4:8], 2)
	repairLeanJournalMarkerCRC(format)
	inputs = append(inputs, format)
	var cases []leanCase
	for index, data := range inputs {
		marker, err := unmarshalJournalGenerationMarker(data, identity)
		var actual any
		if err == nil {
			actual = projectLeanJournalMarker(marker)
		}
		request := map[string]any{"op": "readJournalMarker", "identity": hex.EncodeToString(identity), "input": projectLeanJournalMarkerObservation(data)}
		name := "readJournalMarker seed " + strconv.FormatUint(seed, 10) + " case " + strconv.Itoa(index)
		cases = append(cases, leanCase{name: name, request: marshalLeanJournal(t, request), ok: err == nil, field: "marker", value: marshalLeanJournal(t, actual)})
	}
	return cases
}

// repairLeanJournalMarkerCRC permits structural checks to run independently of checksum rejection.
func repairLeanJournalMarkerCRC(data []byte) {
	binary.BigEndian.PutUint32(data[140:144], crc32.Checksum(data[:140], crc32.MakeTable(crc32.Castagnoli)))
}

// projectLeanJournalMarkerObservation decodes raw fixed-width fields without invoking marker admission.
func projectLeanJournalMarkerObservation(data []byte) any {
	padded := make([]byte, journalGenerationMarkerSize)
	copy(padded, data)
	marker := decodeLeanJournalMarker(data)
	return map[string]any{
		"size": uint64(len(data)), "magic": hex.EncodeToString(padded[:4]), "format": binary.BigEndian.Uint32(padded[4:8]),
		"crc": binary.BigEndian.Uint32(padded[140:144]), "computedCRC": crc32.Checksum(padded[:140], crc32.MakeTable(crc32.Castagnoli)),
		"marker": projectLeanJournalMarker(marker),
	}
}

// projectLeanJournalMarker preserves all marker fields, including the captured retired segment binding.
func projectLeanJournalMarker(marker journalGenerationMarker) any {
	return map[string]any{
		"identity": hex.EncodeToString(marker.Identity), "generation": marker.Generation, "nextSequence": marker.NextSequence,
		"snapshotLength": marker.SnapshotLength, "snapshotDigest": hex.EncodeToString(marker.SnapshotDigest),
		"retiredLength": marker.RetiredLength, "retiredDigest": hex.EncodeToString(marker.RetiredDigest),
	}
}

// decodeLeanJournalMarker decodes fixed-width fields without checking marker admission.
func decodeLeanJournalMarker(data []byte) journalGenerationMarker {
	padded := make([]byte, journalGenerationMarkerSize)
	copy(padded, data)
	return journalGenerationMarker{
		Identity: padded[8:40], Generation: binary.BigEndian.Uint64(padded[40:48]),
		NextSequence: binary.BigEndian.Uint64(padded[48:56]), SnapshotLength: binary.BigEndian.Uint32(padded[56:60]),
		SnapshotDigest: padded[60:92], RetiredLength: binary.BigEndian.Uint64(padded[92:100]), RetiredDigest: padded[100:132],
	}
}
