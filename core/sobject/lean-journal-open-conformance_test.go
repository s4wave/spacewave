package sobject

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/pkg/errors"
)

// TestLeanJournalOpenConformance checks plain, compact and pending recovery with injected storage failures.
func TestLeanJournalOpenConformance(t *testing.T) {
	oracle := leanOracle(t)
	var cases []leanCase
	for seed := range uint64(20) {
		cases = append(cases, runLeanJournalOpenScenario(t, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanJournalOpen searches checkpoint selection, authentication and tail truncation outcomes.
func FuzzLeanJournalOpen(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanJournalOpenScenario(t, seed))
	})
}

// openFaultStorage counts Size calls independently of marker and segment reads.
type openFaultStorage struct {
	*activationFaultStorage
	sizeCalls    int
	failSizeCall int
}

func (s *openFaultStorage) Size() (int64, error) {
	s.sizeCalls++
	if s.sizeCalls == s.failSizeCall {
		return 0, errors.New("injected recovery size failure")
	}
	return s.memoryJournalStorage.Size()
}

// runLeanJournalOpenScenario projects primitive decoding before invoking the real opener.
func runLeanJournalOpenScenario(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	scope := testScope("lean open " + strconv.FormatUint(seed, 10))
	crypto := testJournalCrypto(t, scope)
	version := JournalVersion(seed%17+1, seed%13+1, 1, testDigest("config"))
	var cases []leanCase
	for mode := range 7 {
		for variant := range 24 {
			storage := &openFaultStorage{activationFaultStorage: &activationFaultStorage{
				publicationFaultStorage: &publicationFaultStorage{memoryJournalStorage: newMemoryJournalStorage()},
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
			}
			if mode >= 2 {
				if mode == 3 || mode == 6 {
					storage.fault, storage.armed = 4, true
					if mode == 6 {
						storage.fault = 11
					}
				}
				err := pipeline.journal.checkpoint()
				if (err != nil) != (mode == 3 || mode == 6) {
					t.Fatalf("prepare mode %d: %v", mode, err)
				}
				storage.armed = false
			}
			if mode == 1 || mode == 2 || mode == 5 {
				key := testMutationKey(scope, "peer", "tail")
				record := testIntent(t, crypto, key, testLineage(key, nil), version, 3, "tail")
				if mode == 2 {
					if err := pipeline.appendRecord(record); err != nil {
						t.Fatal(err)
					}
				} else {
					frame, err := marshalJournalFrame(record.Kind, 3, mustMarshalVT(t, record))
					if err != nil {
						t.Fatal(err)
					}
					cut := 1 + int((seed*73+11)%uint64(len(frame)-1))
					storage.data = append(storage.data, frame[:cut]...)
					if err := storage.Sync(); err != nil {
						t.Fatal(err)
					}
				}
			}
			activeCrypto := crypto
			var target JournalStorage = storage
			storageAvailable, generationSupported, floorSupported := true, true, true
			floorReadOK, scanSizeOK, tailSizeOK, retiredReadOK := true, true, true, true
			fault := 0
			switch variant {
			case 1:
				activeCrypto = nil
			case 2:
				activeCrypto = testJournalCrypto(t, testScope("another account"))
			case 3:
				target, storageAvailable = nil, false
			case 4:
				target, generationSupported = struct{ JournalStorage }{storage}, false
			case 5:
				target = activationWithoutFloor{JournalStorage: storage, JournalGenerationStore: storage}
				floorSupported = false
			case 6:
				storage.identity = nil
			case 7:
				storage.markerReadErr = true
			case 8:
				floorReadOK = false
				storage.setGenerationFloorReadFailure(errors.New("injected floor read failure"))
			case 9:
				storage.failSizeCall, scanSizeOK = 1, false
			case 10:
				storage.failSizeCall, tailSizeOK, retiredReadOK = 2, false, false
			case 11:
				storage.retiredReadErr, retiredReadOK = true, false
			case 12, 13, 14:
				fault = variant - 11
				storage.armed = true
				storage.fault = []int{0, 8, 11, 9}[fault]
			case 15:
				storage.generationFloor = decodeLeanJournalMarker(storage.marker).Generation + 1
			case 16:
				storage.marker = nil
			case 17:
				if len(storage.marker) != 0 {
					storage.marker[0] ^= 1
				}
			case 18:
				delete(storage.generations, decodeLeanJournalMarker(storage.marker).Generation)
			case 19:
				ciphertext := storage.generations[decodeLeanJournalMarker(storage.marker).Generation]
				if len(ciphertext) != 0 {
					ciphertext[len(ciphertext)-1] ^= 1
				}
			case 20:
				if len(storage.marker) != 0 {
					marker := decodeLeanJournalMarker(storage.marker)
					plaintext, err := crypto.OpenCheckpointGeneration(storage.generations[marker.Generation], storage.identity, marker.Generation, marker.NextSequence, int(marker.SnapshotLength), marker.SnapshotDigest)
					if err != nil {
						t.Fatal(err)
					}
					var checkpoint SOJournalCheckpoint
					if err := checkpoint.UnmarshalVT(plaintext); err != nil {
						t.Fatal(err)
					}
					checkpoint.NextSequence++
					plaintext = mustMarshalVT(t, &checkpoint)
					encrypted, err := crypto.SealCheckpointGeneration(storage.identity, marker.Generation, marker.NextSequence, plaintext)
					if err != nil {
						t.Fatal(err)
					}
					storage.generations[marker.Generation] = encrypted
					digest := sha256.Sum256(plaintext)
					binary.BigEndian.PutUint32(storage.marker[56:60], uint32(len(plaintext))) //nolint:gosec // The bounded fixture is smaller than a checkpoint frame.
					copy(storage.marker[60:92], digest[:])
					repairLeanJournalMarkerCRC(storage.marker)
				}
			case 21:
				if len(storage.data) != 0 {
					storage.data[len(storage.data)/2] ^= 1
				}
			case 22:
				storage.data = append(storage.data, 0xff)
			case 23:
				if mode == 3 {
					storage.data = storage.data[:len(storage.data)-1]
				}
			}
			storage.sizeCalls = 0
			data := storage.bytes()
			name := " seed " + strconv.FormatUint(seed, 10) + " mode " + strconv.Itoa(mode) + " variant " + strconv.Itoa(variant)
			cases = append(cases, leanJournalFrameByteCases(t, data, name)...)
			frames := projectLeanJournalFrames(t, data)
			if storage.retiredReadErr {
				for _, frame := range frames {
					frame.(map[string]any)["readOK"] = false
				}
			}
			var marker, checkpoint any
			var attempts []*JournalAttemptSnapshot
			if len(storage.marker) != 0 {
				marker = projectLeanJournalMarkerObservation(storage.marker)
				if activeCrypto != nil {
					fields := decodeLeanJournalMarker(storage.marker)
					plaintext, openErr := activeCrypto.OpenCheckpointGeneration(storage.generations[fields.Generation], storage.identity, fields.Generation, fields.NextSequence, int(fields.SnapshotLength), fields.SnapshotDigest)
					if openErr == nil {
						checkpoint = projectLeanJournalCheckpoint(t, plaintext)
						var decoded SOJournalCheckpoint
						if decoded.UnmarshalVT(plaintext) == nil {
							for _, attempt := range decoded.Attempts {
								attempts = append(attempts, decodeLeanCheckpointAttempt(attempt))
							}
						}
					}
				}
			}
			retiredDigest := sha256.Sum256(data)
			input := map[string]any{
				"bytes":            map[string]any{"data": leanJournalBytes(data), "durable": leanJournalBytes(storage.durable)},
				"storageAvailable": storageAvailable, "generationSupported": generationSupported, "floorSupported": floorSupported,
				"identity": hex.EncodeToString(storage.identity), "crypto": activeCrypto != nil,
				"markerReadOK": !storage.markerReadErr, "marker": marker, "floorReadOK": floorReadOK, "floor": storage.generationFloor,
				"checkpoint": checkpoint, "scanSizeOK": scanSizeOK, "frames": projectLeanJournalFramePrimitives(frames), "retiredReadOK": retiredReadOK,
				"retiredDigest": hex.EncodeToString(retiredDigest[:]), "tailSizeOK": tailSizeOK, "fault": int32(fault),
			}
			auth := projectLeanRetainedAuthentication(t, decodeLeanJournalRecords(data), attempts, activeCrypto, storage.identity)
			request := map[string]any{"op": "openJournalWriter", "input": input, "auth": auth}
			writer, _, openErr := openJournalWriter(target, activeCrypto)
			var projectedWriter any
			if openErr == nil {
				projectedWriter = projectLeanJournalWriter(t, writer, storage.memoryJournalStorage)
			}
			actual := map[string]any{"writer": projectedWriter, "bytes": map[string]any{"data": leanJournalBytes(storage.bytes()), "durable": leanJournalBytes(storage.durable)}}
			cases = append(cases, leanCase{name: "openJournalWriter" + name, request: marshalLeanJournal(t, request), ok: openErr == nil, field: "opened", value: marshalLeanJournal(t, actual)})
		}
	}
	return cases
}

// decodeLeanJournalRecords decodes complete protobuf payloads without validating headers or records.
func decodeLeanJournalRecords(data []byte) []*SOJournalRecord {
	var records []*SOJournalRecord
	for len(data) >= journalHeaderSize {
		length := uint64(binary.BigEndian.Uint32(data[16:20]))
		end := uint64(journalHeaderSize) + length
		if end+journalTrailerSize > uint64(len(data)) {
			break
		}
		record := &SOJournalRecord{}
		if record.UnmarshalVT(projectLeanJournalPlaintext(data[journalHeaderSize:end])) == nil {
			records = append(records, record)
		}
		data = data[end+journalTrailerSize:]
	}
	return records
}

// leanJournalFrameByteCases compares the raw slicing and emitted bytes used by the frame roundtrip proof.
func leanJournalFrameByteCases(t *testing.T, data []byte, name string) []leanCase {
	t.Helper()
	var cases []leanCase
	frames := projectLeanJournalFrames(t, data)
	if len(frames) != 0 {
		frame := frames[0].(map[string]any)
		request := map[string]any{
			"op": "observeJournalFrame", "bytes": leanJournalBytes(data), "record": frame["record"],
			"headerCRC": frame["headerCRC"], "frameCRC": frame["frameCRC"],
		}
		cases = append(cases, leanCase{name: "observeJournalFrame" + name, request: marshalLeanJournal(t, request), ok: true, field: "frame", value: marshalLeanJournal(t, frame)})
	}
	records := decodeLeanJournalRecords(data)
	if len(records) != 0 {
		record := records[0]
		frame, err := marshalJournalFrame(record.Kind, record.Sequence, mustMarshalVT(t, record))
		var actual any
		if err == nil {
			actual = leanJournalBytes(frame)
		}
		request := map[string]any{"op": "encodeJournalFrame", "record": projectLeanJournalRecord(t, record), "encoding": projectLeanJournalEncoding(t, record)}
		cases = append(cases, leanCase{name: "encodeJournalFrame" + name, request: marshalLeanJournal(t, request), ok: err == nil, field: "bytes", value: marshalLeanJournal(t, actual)})
	}
	return cases
}
