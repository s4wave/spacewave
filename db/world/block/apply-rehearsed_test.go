package world_block_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
)

// TestApplyRehearsedRejectsWithoutSlot checks that an operation the rehearsal
// rejects returns while another write holds the writer slot, and that one it
// accepts waits for the slot and applies.
func TestApplyRehearsedRejectsWithoutSlot(t *testing.T) {
	// Create the object the accepted operation updates.
	ctx := t.Context()
	eng, cleanup := setupWorldWriteBench(ctx, t)
	defer cleanup()
	ws := world.NewEngineWorldState(eng, true)
	createWorldWriteBenchObject(ctx, t, ws, "rehearsed/present")

	// Hold the writer slot.
	holder, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Discard()

	// An operation on a missing object is rejected without the slot.
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, _, err = world.ApplyRehearsed(rctx, ws, world_mock.NewMockWorldOp("rehearsed/missing", "next"), "")
	if !errors.Is(err, world.ErrObjectNotFound) || rctx.Err() != nil {
		t.Fatalf("rejected operation returned %v", err)
	}

	// An accepted operation waits for the slot, then applies.
	done := make(chan error, 1)
	go func() {
		_, _, err := world.ApplyRehearsed(ctx, ws, world_mock.NewMockWorldOp("rehearsed/present", "next"), "")
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("operation applied while the slot was held: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	holder.Discard()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestSqlCommitRehearsed measures the writer slot held by a SQL commit, the
// first after its World opens on a cold cache, on a World of production size,
// with the rehearsal SQL commits run and without.
func TestSqlCommitRehearsed(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds a World of production size")
	}

	// Seed the World and a SQL database with one table.
	ctx := t.Context()
	tbed, rootRef, err := seedWorld(ctx, worldSeedObjects)
	if err != nil {
		t.Fatal(err)
	}
	defer tbed.Release()
	rootRef = seedSqlTable(ctx, t, tbed, rootRef)

	// Commit one row on a fresh engine per trial.
	const trials = 15
	const coldRead = 200 * time.Microsecond
	run := func(rehearse bool) (time.Duration, int64) {
		holds := make([]time.Duration, 0, trials)
		var reads int64
		for i := range trials {
			eng, cleanup := openSeededEngine(ctx, t, tbed, rootRef)
			cold := newColdCache(coldRead)
			world_block.WrapBlockStore(eng, cold.readThrough())
			timer := &slotTimer{Engine: eng, cold: cold}
			var target world.Engine = timer
			if rehearse {
				target = &rehearsingSlotTimer{slotTimer: timer, re: eng}
			}
			store, err := s4wave_sql_world.NewWorldBackedSql(ctx, world.NewEngineWorldState(target, true), worldSqlKey)
			if err != nil {
				t.Fatal(err)
			}
			execSqlCommit(ctx, t, store, "INSERT INTO shop.items (id) VALUES ("+strconv.Itoa(i)+")")
			holds = append(holds, timer.hold)
			reads += cold.slotReads.Load()
			store.Close()
			cleanup()
		}
		slices.Sort(holds)
		return holds[len(holds)/2], reads
	}
	baseHold, baseReads := run(false)
	hold, reads := run(true)
	t.Logf("without rehearsal: median slot hold %v, %d cold reads under the slot", baseHold, baseReads)
	t.Logf("with rehearsal: median slot hold %v, %d cold reads under the slot", hold, reads)
	if reads != 0 {
		t.Fatalf("SQL commits read %d cold blocks under the slot after rehearsal", reads)
	}
	if hold >= baseHold {
		t.Fatalf("median slot hold %v did not fall from %v", hold, baseHold)
	}
}

// worldSqlKey is the seeded SQL database object.
const worldSqlKey = "bench/seeded/sql"

// seedSqlTable adds a SQL database object with an items table to the World at
// rootRef and returns the new root.
func seedSqlTable(ctx context.Context, t *testing.T, tbed *testbed.Testbed, rootRef *bucket.ObjectRef) *bucket.ObjectRef {
	// Create the typed database object.
	t.Helper()
	eng, cleanup := openSeededEngine(ctx, t, tbed, rootRef)
	defer cleanup()
	ws := world.NewEngineWorldState(eng, true)
	obj, _, err := world.CreateWorldObject(ctx, ws, worldSqlKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(sql_mysql.NewRootBlock(), true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, worldSqlKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatal(err)
	}

	// Create the database and its table.
	store, err := s4wave_sql_world.NewWorldBackedSql(ctx, ws, worldSqlKey)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	execSqlCommit(ctx, t, store, "CREATE DATABASE shop")
	execSqlCommit(ctx, t, store, "CREATE TABLE shop.items (id BIGINT NOT NULL PRIMARY KEY)")
	return eng.GetRootRef()
}

// execSqlCommit runs query in its own SQL write transaction and commits it.
func execSqlCommit(ctx context.Context, t *testing.T, store *s4wave_sql_world.WorldBackedSql, query string) {
	// Open the transaction and run the query.
	t.Helper()
	tx, err := store.NewSqlTransaction(ctx, true, "")
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	ops, err := tx.GetSqlOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ops.ExecContext(ctx, query, nil); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// slotTimer is an Engine that adds up how long its write transactions hold
// the writer slot and marks the cold cache while they do.
type slotTimer struct {
	world.Engine
	// cold counts the cold reads under the slot.
	cold *coldCache
	// hold is the total time write transactions held the slot.
	hold time.Duration
}

// NewTransaction opens a transaction, timing a write until it ends.
func (s *slotTimer) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := s.Engine.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	s.cold.inSlot.Store(true)
	return &timedTx{Tx: tx, s: s, start: time.Now()}, nil
}

// timedTx is a write transaction whose end stops its slotTimer.
type timedTx struct {
	world.Tx
	// s is the timer.
	s *slotTimer
	// start is when the slot was taken.
	start time.Time
	// ended is set once the hold is added.
	ended bool
}

// Commit commits and stops the timer.
func (t *timedTx) Commit(ctx context.Context) error {
	defer t.end()
	return t.Tx.Commit(ctx)
}

// Discard discards and stops the timer.
func (t *timedTx) Discard() {
	t.Tx.Discard()
	t.end()
}

// end adds the hold once.
func (t *timedTx) end() {
	if t.ended {
		return
	}
	t.ended = true
	t.s.hold += time.Since(t.start)
	t.s.cold.inSlot.Store(false)
}

// rehearsingSlotTimer is a slotTimer whose Engine can rehearse.
type rehearsingSlotTimer struct {
	*slotTimer
	// re opens the rehearsals.
	re world.RehearsalEngine
}

// NewRehearsalTransaction opens a rehearsal on the timed Engine.
func (s *rehearsingSlotTimer) NewRehearsalTransaction(ctx context.Context) (world.Tx, error) {
	return s.re.NewRehearsalTransaction(ctx)
}
