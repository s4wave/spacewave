package s4db

import (
	"path/filepath"
	"slices"
	"testing"
)

// TestFlushKeepsReplayChunks replays a log whose flush records follow links,
// and checks that a flush leaves the chunks its superblock replays from as
// log chunks and makes only the chunks before its replay point pending.
func TestFlushKeepsReplayChunks(t *testing.T) {
	// Start from an empty file's superblock: the log in chunk 2.
	m := newManifest()
	*m.table.at(1) = chunkEntry{kind: chunkManifest, owner: 1}
	*m.table.at(2) = chunkEntry{kind: chunkLog}
	sb := superblock{logPos: chunkStart(2), logEnd: chunkStart(3), memPos: chunkStart(2), memEnd: chunkStart(3)}
	st := newState(sb, m, 1<<20)
	apply := func(r *record) {
		r.size = 100
		st.apply(r, 1<<20)
	}

	// A commit, a link into chunk 3, a commit there, then a flush through
	// the first commit: chunk 2 holds the records the flush replays.
	apply(&record{seq: 1, kind: kindCommit})
	replay := st.point()
	apply(&record{seq: 2, kind: kindLink, next: 3, span: 1})
	apply(&record{seq: 3, kind: kindCommit})
	apply(&record{seq: 4, kind: kindFlush, through: 1, replay: replay, runs: []runMeta{{id: 16, chunks: []uint64{10}}}, mchunks: []uint64{11}})
	if k := st.m.table.at(2).kind; k != chunkLog {
		t.Fatalf("chunk 2 is kind %d after the flush, want log", k)
	}
	if !slices.Equal(st.logChunks, []uint64{2, 3}) {
		t.Fatalf("log chunks %v, want [2 3]", st.logChunks)
	}
	if got := st.superblock(); got.through != 1 || got.memPos != replay.pos {
		t.Fatalf("superblock through %d memPos %d, want 1 and %d", got.through, got.memPos, replay.pos)
	}

	// A link into chunk 4 and a flush through the commit in chunk 3: chunk 2
	// is pending from the flush, chunks 3 and 4 stay.
	replay = st.point()
	apply(&record{seq: 5, kind: kindLink, next: 4, span: 1})
	apply(&record{seq: 6, kind: kindFlush, through: 3, replay: replay, runs: []runMeta{{id: 96, chunks: []uint64{12}}}, mchunks: []uint64{13}})
	if e := st.m.table.at(2); e.kind != chunkPending || e.seq != 6 {
		t.Fatalf("chunk 2 is %+v after the second flush, want pending at 6", e)
	}
	for _, c := range []uint64{3, 4} {
		if k := st.m.table.at(c).kind; k != chunkLog {
			t.Fatalf("chunk %d is kind %d, want log", c, k)
		}
	}
	if !slices.Equal(st.logChunks, []uint64{3, 4}) {
		t.Fatalf("log chunks %v, want [3 4]", st.logChunks)
	}
}

// TestPick checks the merge pick. A level 0 run at the top of the file never
// moves down alone: the pick takes the whole level, which keeps the newest
// version of every key above the older ones. Level 0 competes with the lower
// levels by how far each is over its target, so a steady stream of flushes
// does not starve them.
func TestPick(t *testing.T) {
	// Two level 0 runs with a free chunk between them, the newer on top.
	m := newManifest()
	m.runs = []runMeta{
		{id: 16, level: 0, chunks: []uint64{5}, smallest: []byte("a"), largest: []byte("z")},
		{id: 32, level: 0, chunks: []uint64{7}, smallest: []byte("a"), largest: []byte("z")},
	}
	*m.table.at(5) = chunkEntry{kind: chunkRun, owner: 16}
	*m.table.at(7) = chunkEntry{kind: chunkRun, owner: 32}
	pick, ok := (&mergeJob{}).pick(m)
	if !ok || len(pick.inputs) != 2 || pick.level != 1 {
		t.Fatalf("pick %+v ok %v, want both level 0 runs into level 1", pick, ok)
	}

	// The top run at level 1 goes down alone with what it overlaps.
	m.runs[1].level = 1
	pick, ok = (&mergeJob{}).pick(m)
	if !ok || len(pick.inputs) != 1 || pick.inputs[0].id != 32 || pick.level != 2 {
		t.Fatalf("pick %+v ok %v, want run 32 into level 2", pick, ok)
	}

	// Level 0 over its trigger yields to a level 1 further over its target,
	// and takes its turn once it is the further over.
	m = newManifest()
	for i := range 6 {
		m.runs = append(m.runs, runMeta{id: uint64(16 * (i + 1)), level: 0, smallest: []byte("a"), largest: []byte("z")}) // #nosec G115 -- a small count.
	}
	m.runs = append(m.runs, runMeta{id: 1000, level: 1, chunks: make([]uint64, 3*runTarget*levelRatio/chunkSize), smallest: []byte("a"), largest: []byte("z")})
	pick, ok = (&mergeJob{}).pick(m)
	if !ok || len(pick.inputs) != 1 || pick.inputs[0].id != 1000 || pick.level != 2 {
		t.Fatalf("pick %+v ok %v, want the level 1 run into level 2", pick, ok)
	}
	for i := range 8 {
		m.runs = append([]runMeta{{id: uint64(200 + 16*i), level: 0, smallest: []byte("a"), largest: []byte("z")}}, m.runs...) // #nosec G115 -- a small count.
	}
	pick, ok = (&mergeJob{}).pick(m)
	if !ok || len(pick.inputs) != 15 || pick.level != 1 {
		t.Fatalf("pick %+v ok %v, want level 0 and level 1 into level 1", pick, ok)
	}
}

// TestWriteSuperFollowsFile checks that a superblock write takes its
// generation from the file, so a process whose copy is stale never writes
// over a newer generation, and skips a superblock older than the file's.
func TestWriteSuperFollowsFile(t *testing.T) {
	// Plant a newer generation at a higher seq, as another process would.
	db := openTest(t, filepath.Join(t.TempDir(), "super.s4wave"), Options{})
	defer db.Close()
	planted := db.super
	planted.gen, planted.seq = 7, 50
	if _, err := db.s.WriteAt(planted.encode(), planted.page()*pageSize); err != nil {
		t.Fatal(err)
	}

	// An older superblock is not written.
	older := planted
	older.seq = 40
	if err := db.writeSuper(older); err != nil {
		t.Fatal(err)
	}
	order, err := db.readSupers()
	if err != nil || order[0].gen != 7 || order[0].seq != 50 {
		t.Fatalf("after an older write: %+v, %v", order, err)
	}

	// A newer one follows the generation and becomes this handle's.
	newer := planted
	newer.seq = 60
	if err := db.writeSuper(newer); err != nil {
		t.Fatal(err)
	}
	if order, err = db.readSupers(); err != nil || order[0].gen != 8 || order[0].seq != 60 || order[1].gen != 7 {
		t.Fatalf("after a newer write: %+v, %v", order, err)
	}
	if db.super.gen != 8 {
		t.Fatalf("db.super gen %d, want 8", db.super.gen)
	}
}
