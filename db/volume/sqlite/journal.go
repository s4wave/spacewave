package volume_sqlite

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"

	kvtx_journal "github.com/s4wave/spacewave/db/kvtx/journal"
)

// JournalWrap returns a NewSqliteWrapped wrapper that journals the Volume's
// commits in sink, as kvtx_journal describes: it replays sink's records past
// the database's durable head before the Volume opens, and runs the journal's
// background flush until the Volume closes.
func JournalWrap(sink kvtx_journal.Sink, opts kvtx_journal.Options) func(context.Context, Store) (Store, error) {
	return func(ctx context.Context, inner Store) (Store, error) {
		// Journal only a store with ordered commits.
		ordered, ok := inner.(kvtx_journal.Inner)
		if !ok {
			_ = inner.Close()
			return nil, errors.New("sqlite store has no ordered commits to journal")
		}

		// Replay, then flush in the background.
		journal, err := kvtx_journal.Open(ctx, ordered, sink, opts)
		if err != nil {
			_ = inner.Close()
			return nil, err
		}
		runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = journal.Run(runCtx)
		}()
		return &journaledStore{Store: journal, inner: inner, stop: func() { cancel(); <-done }}, nil
	}
}

// journaledStore is a journaled SQLite store with the pool and lifetime of
// the database beneath it.
type journaledStore struct {
	*kvtx_journal.Store
	inner Store
	stop  func()
}

// GetDB returns the database's pool.
func (s *journaledStore) GetDB() *sql.DB {
	return s.inner.GetDB()
}

// Execute runs the database's own routine.
func (s *journaledStore) Execute(ctx context.Context) error {
	return s.inner.Execute(ctx)
}

// Close stops the journal's flush, flushes the database, so a clean stop
// needs no replay, and closes it.
func (s *journaledStore) Close() error {
	s.stop()
	_ = s.Store.Close(context.Background())
	return s.inner.Close()
}

// _ is a type assertion
var _ Store = (*journaledStore)(nil)
