package s4db

import (
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

// TestDeltas runs random commits through checkpoints that write deltas and
// merge them in many steps. It checks the contents and the space as the
// deltas grow and merge, opens a copy of the file taken after each durable
// commit as a crash would leave it, reopens mid-round so the round resumes,
// and reopens without deltas so a checkpoint folds them back into the tree.
func TestDeltas(t *testing.T) {
	// Merge a few keys per step, so rounds span many saves.
	prev := mergeStepKeys
	mergeStepKeys = 64
	t.Cleanup(func() { mergeStepKeys = prev })
	dir := t.TempDir()
	path := filepath.Join(dir, "a.s4wave")
	opts := Options{CheckpointMin: 16 << 10, CheckpointMax: 32 << 10, SingleProcess: true, Deltas: true}
	db := openTest(t, path, opts)
	defer func() { _ = db.Close() }()
	r := rand.New(rand.NewPCG(1, 2))
	m := make(model)

	// Commit until merge rounds have run and one was cut by a reopen,
	// checking crash copies along the way.
	var last, rounds, crashes int
	reopened := false
	for i := 0; i < 20000 && (rounds < 10 || !reopened); i++ {
		durable, err := commitRandom(r, db, m, 3000)
		if err != nil {
			t.Fatal(err)
		}
		// A round ends when the deltas drop by more than a fold-in does.
		sb := db.cur.Load().sb
		if len(sb.deltas) < last-1 {
			rounds++
		}
		last = len(sb.deltas)

		// A copy after a durable commit holds every commit so far.
		if durable && i%13 == 0 {
			crashes++
			checkCrashCopy(t, path, filepath.Join(dir, "crash.s4wave"), maps.Clone(m))
		}

		// Reopen once a round is open, so it resumes from its cursor.
		if !reopened && sb.round > 0 && sb.cursor != nil {
			reopened = true
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db = openTest(t, path, opts)
			m.check(t, db)
		}
		if i%251 == 0 {
			m.check(t, db)
			checkSpace(t, db)
		}
	}
	if rounds < 10 || !reopened || crashes == 0 {
		t.Fatalf("ran %d merge rounds, reopened mid-round %v, %d crash copies", rounds, reopened, crashes)
	}
	t.Logf("%d merge rounds, %d crash copies, %d deltas, %d keys", rounds, crashes, last, len(m))
	m.check(t, db)
	checkSpace(t, db)

	// Without deltas the next checkpoint folds them all into the tree.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	opts.Deltas = false
	db = openTest(t, path, opts)
	for len(db.cur.Load().sb.deltas) != 0 {
		randomCommit(t, r, db, m, 3000)
	}
	m.check(t, db)
	checkSpace(t, db)
}

// checkCrashCopy copies the file at path to dst as a crash would leave it
// and checks that it opens holding m.
func checkCrashCopy(t *testing.T, path, dst string, m model) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		t.Fatal(err)
	}
	db := openTest(t, dst, Options{SingleProcess: true, Deltas: true, RelocateBudget: -1})
	m.check(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}
