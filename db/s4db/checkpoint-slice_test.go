package s4db

import (
	"context"
	"maps"
	"math/rand/v2"
	"path/filepath"
	"testing"
	"time"
)

// TestCheckpointSlices drains a large replayed overlay in slices while
// commits, during a slice's build and between its build and its save,
// rewrite and delete keys it saves and delete keys it skips. It checks the contents and the space after the drain and
// after reopening, and recovers crash copies taken inside slices, so a slice
// that drops a later entry, replays from the wrong record, frees a page a
// later entry needs or miscounts the keys fails here.
func TestCheckpointSlices(t *testing.T) {
	// Build a large overlay with checkpoints held off, then reopen with small
	// slices so the open replays it into one overlay.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "slices.s4wave")
	r := rand.New(rand.NewPCG(3, 4)) // #nosec G404 -- test data.
	m := model{}
	db := openTest(t, path, Options{CheckpointMin: 64 << 20, CheckpointMax: 64 << 20})
	for range 120 {
		randomCommit(t, r, db, m, 1000)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, Options{CheckpointMin: 8 << 10, CheckpointMax: 8 << 10, CheckpointSliceBytes: 4 << 10})

	// Between a slice's build and its save, rewrite a key it saved, delete a
	// key it skipped, and commit at random, in the first slices. Take crash
	// copies before a build and before a save of different slices.
	type crash struct {
		b []byte
		m model
	}
	var crashes []crash
	var slices, builds int
	var hookErr error
	snap := func() {
		b, err := crashImage(db, path)
		if err != nil {
			hookErr = err
			return
		}
		crashes = append(crashes, crash{b, maps.Clone(m)})
	}
	db.w.beforeBuild = func() {
		builds++
		if builds == 2 {
			snap()
		}
		if hookErr == nil && builds <= 6 {
			hookErr = commitOldest(db, m)
		}
	}
	db.w.beforeSave = func(tr *tree) {
		if hookErr != nil || !tr.slice() {
			return
		}
		if slices++; slices > 6 {
			return
		}
		if slices == 3 {
			snap()
		}
		if err := commitSliceRace(db, tr, m); err != nil {
			hookErr = err
			return
		}
		for range 2 {
			if _, err := commitRandom(r, db, m, 1000); err != nil {
				hookErr = err
				return
			}
		}
		if slices == 4 {
			snap()
		}
	}

	// Commit once to start the first slice, and wait for the drain.
	randomCommit(t, r, db, m, 1000)
	wctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	err := db.bcast.Wait(wctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return !db.w.building.Load() && !db.opts.checkpointDue(db.cur.Load()), nil
	})
	if err != nil {
		t.Fatalf("drain: %v after %d slices", err, slices)
	}
	db.w.builds.Wait()
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	t.Logf("%d slices, %d crash copies", slices, len(crashes))
	if slices < 3 || len(crashes) < 3 {
		t.Fatalf("%d slices and %d crash copies, want at least 3 of each", slices, len(crashes))
	}

	// The drained file holds the model, and so does the reopened file.
	m.check(t, db)
	checkSpace(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, Options{})
	m.check(t, db)
	checkSpace(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Every crash copy recovers the model of its moment.
	for i, c := range crashes {
		checkRecovered(t, c.b, path+".crashed", c.m)
		t.Logf("crash copy %d recovered %d keys", i, len(c.m))
	}
}

// commitOldest rewrites the live key with the oldest overlay entry and
// deletes the next oldest, which the next slice saves at their old entries,
// so the overlay published over the slice must keep these later ones. The
// caller does not hold the writer lock.
func commitOldest(db *DB, m model) error {
	// Find the two oldest live entries.
	var old [2]*oentry
	db.cur.Load().overlay.Scan(func(it overlayItem) bool {
		e := it.e
		switch {
		case e.del:
		case old[0] == nil || e.seq < old[0].seq:
			old[0], old[1] = e, old[0]
		case old[1] == nil || e.seq < old[1].seq:
			old[1] = e
		}
		return true
	})
	if old[1] == nil {
		return nil
	}

	// Rewrite the oldest and delete the next.
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	k, v := string(old[0].key), []byte("rewritten during the build")
	if err := tx.Set(ctx, []byte(k), v); err != nil {
		tx.Discard()
		return err
	}
	m[k] = v
	if err := tx.Delete(ctx, old[1].key); err != nil {
		tx.Discard()
		return err
	}
	delete(m, string(old[1].key))
	return tx.Commit(ctx)
}

// commitSliceRace commits, after the cut of slice tr, a rewrite of a key tr
// saved and a delete of a key tr skipped, which the published overlay must
// keep over the tree tr saves. The caller does not hold the writer lock.
func commitSliceRace(db *DB, tr *tree, m model) error {
	// Find a saved set and a skipped live key.
	var saved, skipped string
	for _, ch := range tr.saved {
		if !ch.del {
			saved = string(ch.key)
			break
		}
	}
	db.cur.Load().overlay.Scan(func(it overlayItem) bool {
		if it.e.seq > tr.cut.seq && !it.e.del {
			skipped = string(it.e.key)
			return false
		}
		return true
	})

	// Rewrite the saved key and delete the skipped one.
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	if saved != "" {
		v := []byte("rewritten after the cut " + saved)
		if err := tx.Set(ctx, []byte(saved), v); err != nil {
			tx.Discard()
			return err
		}
		m[saved] = v
	}
	if skipped != "" && skipped != saved {
		if err := tx.Delete(ctx, []byte(skipped)); err != nil {
			tx.Discard()
			return err
		}
		delete(m, skipped)
	}
	return tx.Commit(ctx)
}
