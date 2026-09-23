package sobject

import (
	"crypto/sha256"
	"encoding/binary"
	"hash/crc32"
	"math"
	"strconv"
	"testing"

	"github.com/pkg/errors"
)

// TestLeanJournalStorageConformance compares real frame bytes and injected storage failures.
func TestLeanJournalStorageConformance(t *testing.T) {
	oracle := leanOracle(t)
	var cases []leanCase
	for seed := range uint64(20) {
		cases = append(cases, runLeanJournalStorageScenario(t, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanJournalStorage searches framing cuts, checksums and publication boundaries.
func FuzzLeanJournalStorage(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(math.MaxUint64))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanJournalStorageScenario(t, seed))
	})
}

// runLeanJournalStorageScenario cuts and corrupts journals written by the real encrypted writer.
func runLeanJournalStorageScenario(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	scope := testScope("lean storage " + strconv.FormatUint(seed, 10))
	crypto := testJournalCrypto(t, scope)
	storage := newMemoryJournalStorage()
	writer, _, err := openJournalWriter(storage, crypto)
	if err != nil {
		t.Fatal(err)
	}
	version := JournalVersion(seed%19+1, 1, 1, testDigest("config"))
	for index := range 2 {
		key := testMutationKey(scope, "peer", strconv.Itoa(index))
		if err := writer.Append(testIntent(t, crypto, key, testLineage(key, nil), version, uint64(index+1), "operation")); err != nil {
			t.Fatal(err)
		}
	}
	data := storage.bytes()
	var cases []leanCase
	name := " seed " + strconv.FormatUint(seed, 10)
	for cut := 0; cut <= len(data); cut++ {
		cases = append(cases, leanJournalScanCase(t, data[:cut], 1, name+" cut "+strconv.Itoa(cut)))
	}
	for index := range len(data) {
		corrupt := append([]byte(nil), data...)
		corrupt[index] ^= byte(seed%255 + 1)
		cases = append(cases, leanJournalScanCase(t, corrupt, 1, name+" corrupt "+strconv.Itoa(index)))
	}
	for cut := 1; cut < journalHeaderSize; cut++ {
		for index := range cut {
			corrupt := append([]byte(nil), data[:cut]...)
			corrupt[index] ^= byte(seed%255 + 1)
			cases = append(cases, leanJournalScanCase(t, corrupt, 1, name+" partial "+strconv.Itoa(cut)+" byte "+strconv.Itoa(index)))
		}
	}
	for _, initial := range []uint64{0, 2, 3, math.MaxUint64} {
		cases = append(cases, leanJournalScanCase(t, data, initial, name+" initial "+strconv.FormatUint(initial, 10)))
	}

	// Repair header checksums after changing independent framing fields.
	for variant := range 8 {
		corrupt := append([]byte(nil), data...)
		switch variant {
		case 0:
			binary.BigEndian.PutUint16(corrupt[4:6], 1)
		case 1:
			binary.BigEndian.PutUint16(corrupt[6:8], 0)
		case 2:
			binary.BigEndian.PutUint16(corrupt[6:8], 12)
		case 3:
			binary.BigEndian.PutUint32(corrupt[16:20], journalMaxFramePayload+1)
		case 4:
			binary.BigEndian.PutUint32(corrupt[16:20], math.MaxUint32)
		case 5:
			binary.BigEndian.PutUint32(corrupt[16:20], uint32(len(data)))
		case 6:
			binary.BigEndian.PutUint32(corrupt[16:20], 0)
		case 7:
			binary.BigEndian.PutUint64(corrupt[8:16], 2)
		}
		binary.BigEndian.PutUint32(corrupt[20:24], crc32.Checksum(corrupt[:20], crc32.MakeTable(crc32.Castagnoli)))
		cases = append(cases, leanJournalScanCase(t, corrupt, 1, name+" header "+strconv.Itoa(variant)))
	}
	// A valid unknown field can contain the marker in plaintext, including at a torn-write boundary.
	trailerKey := testMutationKey(scope, "peer", "trailer")
	trailerRecord := testIntent(t, crypto, trailerKey, testLineage(trailerKey, nil), version, 3, "trailer")
	cut := addJournalPayloadTrailer(t, trailerRecord)
	trailerFrame, err := marshalJournalFrame(trailerRecord.Kind, 3, mustMarshalVT(t, trailerRecord))
	if err != nil {
		t.Fatal(err)
	}
	cases = append(cases, leanJournalScanCase(t, append(append([]byte(nil), data...), trailerFrame[:cut]...), 1, name+" payload trailer cut"))
	cases = append(cases, leanJournalScanCase(t, append(append([]byte(nil), data...), trailerFrame...), 1, name+" payload trailer complete"))
	cases = append(cases, leanJournalPayloadCases(t, seed)...)
	cases = append(cases, leanJournalMemoryCases(t, data, seed)...)
	cases = append(cases, leanJournalGenerationCases(t, crypto, seed)...)
	return cases
}

// leanJournalScanCase compares scanner errors, accepted records and exact truncation offset.
func leanJournalScanCase(t *testing.T, data []byte, initial uint64, name string) leanCase {
	t.Helper()
	storage := newMemoryJournalStorage()
	if _, err := storage.WriteAt(data, 0); err != nil {
		t.Fatal(err)
	}
	records, offset, err := scanJournalFrom(storage, initial)
	code := int32(0)
	if err != nil {
		code = 1
		if errors.Is(err, errJournalSequenceBaseMismatch) {
			code = 2
		}
	}
	projected := make([]any, len(records))
	for index, record := range records {
		projected[index] = projectLeanJournalRecord(t, record)
	}
	request := map[string]any{"op": "scanJournalFrames", "initial": initial, "frames": projectLeanJournalFrames(t, data)}
	result := map[string]any{"code": code, "records": projected, "offset": uint64(offset)}
	return leanCase{name: "scanJournalFrames" + name, request: marshalLeanJournal(t, request), ok: err == nil, field: "scan", value: marshalLeanJournal(t, result)}
}

// projectLeanJournalFrames decodes byte boundaries and protobuf only, without calling Go admission.
func projectLeanJournalFrames(t *testing.T, data []byte) []any {
	t.Helper()
	frames := make([]any, 0)
	table := crc32.MakeTable(crc32.Castagnoli)
	for len(data) != 0 {
		header := data[:min(len(data), journalHeaderSize)]
		observation := map[string]any{
			"header": leanJournalBytes(header), "remaining": uint64(len(data)),
			"headerCRC": crc32.Checksum(header[:min(len(header), 20)], table),
			"frameCRC":  uint32(0), "trailer": []any{}, "payload": []any{}, "record": nil, "readOK": true,
		}
		frames = append(frames, observation)
		if len(header) < journalHeaderSize {
			break
		}
		length := uint64(binary.BigEndian.Uint32(header[16:20]))
		end := uint64(journalHeaderSize) + length
		if end+journalTrailerSize > uint64(len(data)) {
			if len(data) >= journalHeaderSize+journalTrailerSize {
				observation["trailer"] = leanJournalBytes(data[len(data)-journalTrailerSize:])
			}
			break
		}
		payload := data[journalHeaderSize:end]
		crc := crc32.New(table)
		_, _ = crc.Write(header[:24])
		_, _ = crc.Write(payload)
		observation["frameCRC"] = crc.Sum32()
		observation["payload"] = leanJournalBytes(payload)
		observation["trailer"] = leanJournalBytes(data[end : end+journalTrailerSize])
		record := &SOJournalRecord{}
		if record.UnmarshalVT(projectLeanJournalPlaintext(payload)) == nil {
			observation["record"] = projectLeanJournalRecord(t, record)
		}
		data = data[end+journalTrailerSize:]
	}
	return frames
}

// leanJournalBytes projects exact bytes as integers without base64 or text interpretation.
func leanJournalBytes(data []byte) []any {
	result := make([]any, len(data))
	for index, value := range data {
		result[index] = uint32(value)
	}
	return result
}

// leanJournalMemoryCases exercises the real storage's partial-write and failed-sync behavior.
func leanJournalMemoryCases(t *testing.T, data []byte, seed uint64) []leanCase {
	t.Helper()
	var cases []leanCase
	for variant := range 12 {
		storage := newMemoryJournalStorage()
		if _, err := storage.WriteAt(data[:28], 0); err != nil {
			t.Fatal(err)
		}
		if err := storage.Sync(); err != nil {
			t.Fatal(err)
		}
		before := map[string]any{"data": leanJournalBytes(storage.bytes()), "durable": leanJournalBytes(storage.durable)}
		offset := int64(seed % 40)
		limit := variant*12 - 12
		writeFail := variant < 5
		if writeFail {
			storage.setWriteFailure(limit, errors.New("injected torn write"))
		}
		_, _ = storage.WriteAt(data[:36], offset)
		sync := variant >= 5
		success := variant >= 8
		if sync {
			if !success {
				storage.setSyncFailure(errors.New("injected sync failure"))
			}
			_ = storage.Sync()
		}
		request := map[string]any{"op": "journalMemoryBytes", "storage": before, "offset": uint64(offset), "bytes": leanJournalBytes(data[:36]), "writeFail": writeFail, "writeLimit": int64(limit), "sync": sync, "success": success}
		result := map[string]any{"data": leanJournalBytes(storage.bytes()), "durable": leanJournalBytes(storage.durable)}
		cases = append(cases, leanCase{name: "journalMemoryBytes seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant), request: marshalLeanJournal(t, request), ok: true, field: "storage", value: marshalLeanJournal(t, result)})
	}
	return cases
}

// leanJournalGenerationCases reopens authenticated checkpoints at ordinary and exhausted counters.
func leanJournalGenerationCases(t *testing.T, crypto *JournalCrypto, seed uint64) []leanCase {
	t.Helper()
	var cases []leanCase
	for _, floor := range []uint64{0, 1, 2, math.MaxUint64 - 1, math.MaxUint64} {
		for _, generation := range []uint64{0, 1, 2, 3, math.MaxUint64 - 1, math.MaxUint64} {
			storage := newMemoryJournalStorage()
			buildGeneration := max(generation, 1)
			plaintext, err := marshalCompactJournalCheckpoint(storage.identity, buildGeneration, 1, NewJournalReducer())
			if err != nil {
				t.Fatal(err)
			}
			encrypted, err := crypto.SealCheckpointGeneration(storage.identity, buildGeneration, 1, plaintext)
			if err != nil {
				t.Fatal(err)
			}
			digest, retired := sha256.Sum256(plaintext), sha256.Sum256(nil)
			marker, err := marshalJournalGenerationMarker(journalGenerationMarker{
				Identity: storage.identity, Generation: buildGeneration, NextSequence: 1,
				SnapshotLength: uint32(len(plaintext)), SnapshotDigest: digest[:], RetiredDigest: retired[:],
			})
			if err != nil {
				t.Fatal(err)
			}
			binary.BigEndian.PutUint64(marker[40:48], generation)
			binary.BigEndian.PutUint32(marker[140:144], crc32.Checksum(marker[:140], crc32.MakeTable(crc32.Castagnoli)))
			storage.marker, storage.generationFloor = marker, floor
			storage.generations[generation] = encrypted
			_, err = OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
			request := map[string]any{"op": "journalGenerationWindow", "floor": floor, "generation": generation}
			name := "journalGenerationWindow seed " + strconv.FormatUint(seed, 10) + " floor " + strconv.FormatUint(floor, 10) + " generation " + strconv.FormatUint(generation, 10)
			cases = append(cases, leanCase{name: name, request: marshalLeanJournal(t, request), ok: err == nil, field: "floor", value: marshalLeanJournal(t, storage.generationFloor)})
		}
	}
	return cases
}

// projectLeanJournalPlaintext reconstructs protobuf input independently of the production decoder.
// The model receives all encoded bytes and checks canonical escapes itself.
func projectLeanJournalPlaintext(encoded []byte) []byte {
	var plaintext []byte
	for len(encoded) != 0 {
		switch encoded[0] {
		case 0:
			if len(encoded) < 2 {
				return nil
			}
			switch encoded[1] {
			case 0:
				plaintext = append(plaintext, 0)
			case 1:
				plaintext = append(plaintext, 'E')
			default:
				return nil
			}
			encoded = encoded[2:]
		case 'E':
			return nil
		default:
			plaintext = append(plaintext, encoded[0])
			encoded = encoded[1:]
		}
	}
	return plaintext
}

// leanJournalPayloadCases exercises every byte and escape code without relying on record admission.
func leanJournalPayloadCases(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	inputs := [][]byte{nil, {0}, {0, 0}, {0, 1}, {0, 0, 1}, []byte("END!")}
	for value := range 256 {
		inputs = append(inputs, []byte{byte(value)}, []byte{0, byte(value)}, []byte{byte(seed), byte(value), 0, 1})
	}
	var cases []leanCase
	for index, input := range inputs {
		decoded, err := unescapeJournalPayload(input)
		var decodedValue any
		if err == nil {
			decodedValue = leanJournalBytes(decoded)
		}
		request := map[string]any{"op": "journalPayloadCodec", "bytes": leanJournalBytes(input)}
		result := map[string]any{"encoded": leanJournalBytes(escapeJournalPayload(input)), "decoded": decodedValue}
		cases = append(cases, leanCase{name: "journalPayloadCodec seed " + strconv.FormatUint(seed, 10) + " case " + strconv.Itoa(index), request: marshalLeanJournal(t, request), ok: err == nil, field: "codec", value: marshalLeanJournal(t, result)})
	}
	return cases
}
