package sobject

import (
	"encoding/binary"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/pkg/errors"
)

// TestLeanJournalPublicationConformance compares every checkpoint publication cut with Lean.
func TestLeanJournalPublicationConformance(t *testing.T) {
	oracle := leanOracle(t)
	var cases []leanCase
	for seed := range uint64(24) {
		cases = append(cases, runLeanJournalPublicationScenario(t, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanJournalPublication searches publication failures across retained generations.
func FuzzLeanJournalPublication(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanJournalPublicationScenario(t, seed))
	})
}

// publicationFaultStorage extends the existing test storage with errors after visible side effects.
type publicationFaultStorage struct {
	*memoryJournalStorage
	fault int
	armed bool
	reads int
}

func (s *publicationFaultStorage) WriteJournalCheckpointGeneration(generation uint64, data []byte) error {
	if s.armed && s.fault == 1 {
		return errors.New("injected candidate write failure")
	}
	if err := s.memoryJournalStorage.WriteJournalCheckpointGeneration(generation, data); err != nil {
		return err
	}
	if s.armed && s.fault == 10 {
		return errors.New("injected candidate sync failure")
	}
	return nil
}

func (s *publicationFaultStorage) WriteJournalGeneration(data []byte) error {
	if s.armed && s.fault == 2 {
		return errors.New("injected marker write failure")
	}
	if err := s.memoryJournalStorage.WriteJournalGeneration(data); err != nil {
		return err
	}
	if s.armed && s.fault == 3 {
		return errors.New("injected marker sync failure")
	}
	return nil
}

func (s *publicationFaultStorage) WriteJournalGenerationFloor(generation uint64) error {
	if s.armed && s.fault == 4 {
		return errors.New("injected floor write failure")
	}
	if err := s.memoryJournalStorage.WriteJournalGenerationFloor(generation); err != nil {
		return err
	}
	if s.armed && s.fault == 5 {
		return errors.New("injected floor sync failure")
	}
	return nil
}

func (s *publicationFaultStorage) ReadAt(data []byte, offset int64) (int, error) {
	n, err := s.memoryJournalStorage.ReadAt(data, offset)
	if s.armed {
		s.reads++
		if s.reads == 2 {
			switch s.fault {
			case 6:
				return 0, errors.New("injected retired segment read failure")
			case 7:
				data[0] ^= 1
			}
		}
	}
	return n, err
}

func (s *publicationFaultStorage) Truncate(size int64) error {
	if s.armed && s.fault == 8 {
		return errors.New("injected truncate failure")
	}
	if err := s.memoryJournalStorage.Truncate(size); err != nil {
		return err
	}
	if s.armed && s.fault == 11 {
		return errors.New("injected failure after truncate")
	}
	return nil
}

func (s *publicationFaultStorage) Sync() error {
	if s.armed && s.fault == 9 {
		s.setSyncFailure(errors.New("injected retirement sync failure"))
	}
	return s.memoryJournalStorage.Sync()
}

// runLeanJournalPublicationScenario checks storage effects and resumes through actual authenticated recovery.
func runLeanJournalPublicationScenario(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	scope := testScope("lean publication " + strconv.FormatUint(seed, 10))
	crypto := testJournalCrypto(t, scope)
	version := JournalVersion(seed%23+1, seed%19+1, 1, testDigest("config"))
	var cases []leanCase
	for fault := range 12 {
		storage := &publicationFaultStorage{memoryJournalStorage: newMemoryJournalStorage(), fault: fault}
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
		before := projectLeanJournalPublication(t, pipeline.journal.writer, storage.memoryJournalStorage)
		generation := pipeline.journal.writer.generation + 1
		storage.armed = true
		err = pipeline.journal.checkpoint()
		request := map[string]any{"op": "publishJournalCheckpoint", "before": before, "generation": generation, "fault": int32(fault)}
		actual := projectLeanJournalPublication(t, pipeline.journal.writer, storage.memoryJournalStorage)
		name := "publishJournalCheckpoint seed " + strconv.FormatUint(seed, 10) + " fault " + strconv.Itoa(fault)
		cases = append(cases, leanCase{name: name, request: marshalLeanJournal(t, request), ok: err == nil, field: "publication", value: marshalLeanJournal(t, actual)})

		// Exercise the old writer before recovery, then require every acknowledged attempt to survive.
		storage.armed = false
		storage.setSyncFailure(nil)
		key := testMutationKey(scope, "peer", "after-publication")
		record := testIntent(t, crypto, key, testLineage(key, nil), version, 3, "after publication")
		appendErr := pipeline.appendRecord(record)
		if err != nil && fault != 1 && fault != 10 && !errors.Is(appendErr, ErrJournalWriterPoisoned) {
			t.Fatalf("%s: incomplete publication did not fence append: %v", name, appendErr)
		}
		if (err == nil || fault == 1 || fault == 10) && appendErr != nil {
			t.Fatalf("%s: usable writer rejected append: %v", name, appendErr)
		}
		acknowledged := pipeline.Snapshot()
		recovered, err := OpenJournalPipelineWithCrypto(storage, crypto, testReceiptVerifier(), testLookupVerifier())
		if err != nil || !reflect.DeepEqual(acknowledged, recovered.Snapshot()) {
			t.Fatalf("%s: acknowledged state did not recover: %v", name, err)
		}
		if appendErr != nil {
			if err := recovered.appendRecord(record); err != nil {
				t.Fatalf("%s: reopened writer did not resume: %v", name, err)
			}
		}
	}
	return cases
}

// projectLeanJournalPublication exposes bytes and numeric metadata independently of admission.
func projectLeanJournalPublication(t *testing.T, writer *journalWriter, storage *memoryJournalStorage) any {
	t.Helper()
	generations := make([]uint64, 0, len(storage.generations))
	for generation := range storage.generations {
		generations = append(generations, generation)
	}
	slices.Sort(generations)
	projectedGenerations := make([]any, len(generations))
	for index, generation := range generations {
		projectedGenerations[index] = generation
	}
	markerGeneration := uint64(0)
	if len(storage.marker) != 0 {
		markerGeneration = binary.BigEndian.Uint64(storage.marker[40:48])
	}
	records := make([]any, len(writer.records))
	for index, record := range writer.records {
		records[index] = projectLeanJournalRecord(t, record)
	}
	return map[string]any{
		"bytes": map[string]any{"data": leanJournalBytes(storage.bytes()), "durable": leanJournalBytes(storage.durable)},
		"floor": storage.generationFloor, "markerGeneration": markerGeneration, "checkpointGenerations": projectedGenerations,
		"generation": writer.generation, "sequence": writer.sequence, "offset": uint64(writer.offset),
		"records": records, "state": projectLeanJournalState(t, writer.reducer.Snapshot()), "poisoned": writer.poisoned != nil,
	}
}
