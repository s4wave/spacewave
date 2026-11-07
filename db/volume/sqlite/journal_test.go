package volume_sqlite_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	kvtx_journal "github.com/s4wave/spacewave/db/kvtx/journal"
	volume_sqlite "github.com/s4wave/spacewave/db/volume/sqlite"
	"github.com/sirupsen/logrus"
)

// memSink is a journal that holds every record.
type memSink struct {
	mtx     sync.Mutex
	records map[uint64]*kvtx_journal.Record
}

func (m *memSink) Connected() bool { return true }

func (m *memSink) Append(_ context.Context, rec *kvtx_journal.Record) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.records[rec.Seq] = rec
	return nil
}

func (m *memSink) Tail(_ context.Context, epoch, after uint64) ([]*kvtx_journal.Record, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	var out []*kvtx_journal.Record
	for seq := after + 1; m.records[seq] != nil && m.records[seq].Epoch == epoch; seq++ {
		out = append(out, m.records[seq])
	}
	return out, nil
}

func (m *memSink) Cut(context.Context, uint64, uint64) error { return nil }

func (m *memSink) Trim(uint64, uint64) {}

func (m *memSink) holds(key string) bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	for _, rec := range m.records {
		for _, w := range rec.Writes {
			if string(w.Key) == key {
				return true
			}
		}
	}
	return false
}

// TestJournaledVolume opens a SQLite Volume through the journal, checks it
// still publishes atomically, journals a live commit, and reopens clean.
func TestJournaledVolume(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	sink := &memSink{records: make(map[uint64]*kvtx_journal.Record)}
	conf := volume_sqlite.NewConfig(filepath.Join(t.TempDir(), "world.db"), "world")
	wrap := volume_sqlite.JournalWrap(sink, kvtx_journal.Options{Live: func() bool { return true }, FlushEvery: 5 * time.Millisecond, Logger: le})

	// Open it; the journal goes live in the background.
	vol, err := volume_sqlite.NewSqliteWrapped(ctx, le, conf, wrap)
	if err != nil {
		t.Fatal(err)
	}
	if !vol.SupportsAtomicPublication() {
		t.Fatal("the journaled Volume lost its atomic publications")
	}
	time.Sleep(50 * time.Millisecond)

	// A write through the Volume reaches the journal once it is live.
	store := vol.GetKvtxStore()
	deadline := time.Now().Add(10 * time.Second)
	for !sink.holds("player") {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("player"), []byte("joined")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("no commit reached the journal")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// A clean close needs no replay and keeps the write.
	reopened, err := volume_sqlite.NewSqlite(ctx, le, conf)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	tx, err := reopened.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if v, found, err := tx.Get(ctx, []byte("player")); err != nil || !found || string(v) != "joined" {
		t.Fatalf("write lost across a clean close: %q %v %v", v, found, err)
	}
}
