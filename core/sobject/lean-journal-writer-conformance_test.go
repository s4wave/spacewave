package sobject

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/pkg/errors"
)

// TestLeanJournalWriterConformance compares durable append effects with the Lean writer.
func TestLeanJournalWriterConformance(t *testing.T) {
	oracle := leanOracle(t)
	var cases []leanCase
	for seed := range uint64(20) {
		cases = append(cases, runLeanJournalWriterScenario(t, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanJournalWriter explores prepared records, authentication and partial writes.
func FuzzLeanJournalWriter(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(math.MaxUint64))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanJournalWriterScenario(t, seed))
	})
}

// runLeanJournalWriterScenario preserves a real acknowledged prefix across each append outcome.
func runLeanJournalWriterScenario(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	scope := testScope("lean writer " + strconv.FormatUint(seed, 10))
	crypto := testJournalCrypto(t, scope)
	version := JournalVersion(seed%19+1, seed%17+1, 1, testDigest("config"))
	var cases []leanCase
	for stage := range 3 {
		for variant := range 30 {
			storage := newMemoryJournalStorage()
			writer, _, err := openJournalWriter(storage, crypto)
			if err != nil {
				t.Fatal(err)
			}
			key := testMutationKey(scope, "peer", "attempt")
			lineage := testLineage(key, nil)
			intent := testIntent(t, crypto, key, lineage, version, 1, "operation")
			decoded := testDecodedIntent(t, crypto, intent, key, lineage, version, 1)
			envelope, err := NewJournalEnvelopeRecord(crypto, 2, decoded, []byte("envelope"), writer.identity)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.Append(intent); err != nil {
				t.Fatal(err)
			}
			other := testMutationKey(scope, "peer", "next")
			record := testIntent(t, crypto, other, testLineage(other, nil), version, 2, "next operation")
			switch stage {
			case 1:
				record = envelope.CloneVT()
			case 2:
				if err := writer.Append(envelope); err != nil {
					t.Fatal(err)
				}
				record = newJournalSentRecord(key, lineage, version)
			}
			writeFail, syncOK, limit := false, true, -1
			switch variant {
			case 1:
				record.Sequence = 0
			case 2:
				record.FormatVersion = 99
			case 3:
				record.Sequence = writer.sequence + 1
			case 4:
				record = nil
			case 5:
				writer.poisoned = ErrJournalWriterPoisoned
			case 6:
				writer.pending = &journalPendingActivation{}
			case 7:
				writer.crypto = nil
			case 8:
				record.Kind = 99
			case 9:
				record.AttemptState = 99
			case 10:
				record.Version = nil
			case 11:
				record.Key = nil
			case 12:
				record.Lineage = nil
			case 13:
				record = intent.CloneVT()
				record.Sequence = writer.sequence
			case 14:
				writer.sequence = 0
				record.Sequence = 0
			case 15:
				writer.sequence = math.MaxUint64
				record.Sequence = 0
			case 16, 17, 18, 19, 20, 21:
				writeFail = true
				limit = []int{-1, 0, 1, journalHeaderSize - 1, journalHeaderSize, int(seed % 400)}[variant-16]
			case 22:
				syncOK = false
			case 23:
				writer.identity = testDigest("another journal")
			case 24:
				switch stage {
				case 0:
					record.Intent.Ciphertext[0] ^= 1
				case 1:
					record.Envelope.Ciphertext[0] ^= 1
				}
			case 25:
				record.EnvelopeDigest = testDigest("wrong envelope")
			case 26, 27, 28, 29:
				if stage == 0 {
					inner := testDecodedIntent(t, crypto, record, record.Key, record.Lineage, record.Version, 2)
					switch variant {
					case 26:
						inner.Key.LocalId = "different"
					case 27:
						inner.Lineage.RootKey = key.CloneVT()
					case 28:
						inner.Lineage.Supersedes = key.CloneVT()
					case 29:
						inner.Version = nil
					}
					record.Intent, err = crypto.SealWithIdentity(record.Kind, 2, record.Key, writer.identity, mustMarshalVT(t, inner))
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			before := projectLeanJournalWriter(t, writer, storage)
			prepared := record.CloneVT()
			if prepared != nil {
				prepared.FormatVersion = JournalFormatVersion
				if prepared.Sequence == 0 {
					prepared.Sequence = writer.sequence
				}
			}
			auth := projectLeanJournalAuthentication(t, prepared, writer.crypto, writer.identity)
			effects := map[string]any{
				"encoding":  projectLeanJournalEncoding(t, prepared),
				"writeFail": writeFail, "writeLimit": int64(limit), "syncOK": syncOK,
			}
			if writeFail {
				storage.setWriteFailure(limit, errors.New("injected append write failure"))
			}
			if !syncOK {
				storage.setSyncFailure(errors.New("injected append sync failure"))
			}
			name := " seed " + strconv.FormatUint(seed, 10) + " stage " + strconv.Itoa(stage) + " variant " + strconv.Itoa(variant)
			if prepared != nil {
				authErr := authenticateJournalRecords([]*SOJournalRecord{prepared}, writer.crypto, writer.identity)
				authRequest := map[string]any{"op": "authenticateJournalRecord", "record": projectLeanJournalRecord(t, prepared), "auth": auth}
				cases = append(cases, leanCase{name: "authenticateJournalRecord" + name, request: marshalLeanJournal(t, authRequest), ok: authErr == nil})
			}
			err = writer.Append(record)
			request := map[string]any{"op": "appendJournalWriter", "before": before, "record": projectLeanJournalRecord(t, record), "auth": auth, "effects": effects}
			cases = append(cases, leanCase{name: "appendJournalWriter" + name, request: marshalLeanJournal(t, request), ok: err == nil, field: "writer", value: marshalLeanJournal(t, projectLeanJournalWriter(t, writer, storage))})

			// Once storage recovers, the acknowledged prefix survives every append failure.
			// Variant 15 deliberately assigns an unreachable sequence to test exhaustion.
			storage.setWriteFailure(0, nil)
			storage.setSyncFailure(nil)
			if variant != 15 {
				recovered, _, openErr := openJournalWriter(storage, crypto)
				if openErr != nil || !reflect.DeepEqual(writer.reducer.Snapshot(), recovered.reducer.Snapshot()) {
					t.Fatalf("%s: acknowledged prefix did not recover: %v", name, openErr)
				}
			}

			// Even after clearing the injected fault, a poisoned writer must reject retry.
			if writer.poisoned != nil {
				beforeRetry := projectLeanJournalWriter(t, writer, storage)
				retryErr := writer.Append(record)
				request["before"] = beforeRetry
				cases = append(cases, leanCase{name: "appendJournalWriter retry" + name, request: marshalLeanJournal(t, request), ok: retryErr == nil, field: "writer", value: marshalLeanJournal(t, projectLeanJournalWriter(t, writer, storage))})
			}
		}
	}
	return cases
}

// projectLeanJournalAuthentication performs primitive decryption/decoding without calling admission.
func projectLeanJournalAuthentication(t *testing.T, record *SOJournalRecord, crypto *JournalCrypto, identity []byte) any {
	t.Helper()
	result := map[string]any{"crypto": crypto != nil, "intent": nil, "envelopeHash": nil}
	if crypto == nil || record == nil {
		return result
	}
	switch record.Kind {
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT:
		plaintext, err := crypto.OpenWithIdentity(record.Kind, record.Sequence, record.Key, identity, record.Intent)
		if err == nil {
			var intent SOJournalIntent
			if intent.UnmarshalVT(plaintext) == nil {
				result["intent"] = map[string]any{"key": projectLeanJournalKey(intent.Key), "lineage": projectLeanJournalLineage(intent.Lineage), "version": projectLeanJournalVersion(t, intent.Version)}
			}
		}
	case SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE:
		plaintext, err := crypto.OpenWithIdentity(record.Kind, record.Sequence, record.Key, identity, record.Envelope)
		if err == nil {
			digest := sha256.Sum256(plaintext)
			result["envelopeHash"] = hex.EncodeToString(digest[:])
		}
	}
	return result
}

// projectLeanJournalEncoding exposes protobuf and checksum primitives, never the Go frame result.
func projectLeanJournalEncoding(t *testing.T, record *SOJournalRecord) any {
	t.Helper()
	result := map[string]any{"payload": nil, "headerCRC": uint32(0), "frameCRC": uint32(0)}
	if record == nil {
		return result
	}
	payload, err := record.MarshalVT()
	if err != nil {
		return result
	}
	result["payload"] = leanJournalBytes(payload)
	encoded := make([]byte, 0, len(payload)*2)
	for _, value := range payload {
		switch value {
		case 0:
			encoded = append(encoded, 0, 0)
		case 'E':
			encoded = append(encoded, 0, 1)
		default:
			encoded = append(encoded, value)
		}
	}
	header := make([]byte, 24)
	copy(header, []byte("SWJ1"))
	binary.BigEndian.PutUint16(header[4:6], 2)
	binary.BigEndian.PutUint16(header[6:8], uint16(record.Kind)) //nolint:gosec // Mirrors the uint16 wire representation even for adversarial enum codes.
	binary.BigEndian.PutUint64(header[8:16], record.Sequence)
	binary.BigEndian.PutUint32(header[16:20], uint32(len(encoded))) //nolint:gosec // Test fixtures remain far below the frame size limit.
	table := crc32.MakeTable(crc32.Castagnoli)
	headerCRC := crc32.Checksum(header[:20], table)
	binary.BigEndian.PutUint32(header[20:24], headerCRC)
	crc := crc32.New(table)
	_, _ = crc.Write(header)
	_, _ = crc.Write(encoded)
	result["headerCRC"], result["frameCRC"] = headerCRC, crc.Sum32()
	return result
}

// projectLeanJournalWriter retains the entire writer state and both visible/durable byte sequences.
func projectLeanJournalWriter(t *testing.T, writer *journalWriter, storage *memoryJournalStorage) any {
	t.Helper()
	records := make([]any, len(writer.records))
	for index, record := range writer.records {
		records[index] = projectLeanJournalRecord(t, record)
	}
	var pending any
	if writer.pending != nil {
		pending = map[string]any{"marker": projectLeanJournalMarker(writer.pending.marker), "floor": writer.pending.floor}
	}
	return map[string]any{
		"bytes":    map[string]any{"data": leanJournalBytes(storage.bytes()), "durable": leanJournalBytes(storage.durable)},
		"sequence": writer.sequence, "offset": uint64(writer.offset), "records": records,
		"state": projectLeanJournalState(t, writer.reducer.Snapshot()), "poisoned": writer.poisoned != nil, "pending": pending,
		"identity": hex.EncodeToString(writer.identity), "generation": writer.generation,
	}
}
