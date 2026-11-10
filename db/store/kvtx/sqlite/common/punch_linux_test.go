//go:build linux && cgo

package common_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/s4wave/spacewave/db/sqlitefree"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/cgo"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// punchRows is how many rows a punch test writes; a value fills most of a
// 4096 byte page, so each row takes a table leaf of its own.
const punchRows, punchValueSize = 6000, 3000

// punchKey and punchValue are row i of a punch test.
func punchKey(i int) []byte { return fmt.Appendf(nil, "k%06d", i) }

// punchValue is the value of row i, distinct per row.
func punchValue(i int) []byte {
	return bytes.Repeat(fmt.Appendf(nil, "%06d.", i), punchValueSize/7)
}

// punchEnv sets the punch's environment for a store opened after: on or
// off, keeping reserve bytes, with an interval long enough that only the
// test runs rounds.
func punchEnv(t *testing.T, on bool, reserve int64) {
	t.Helper()
	val := ""
	if on {
		val = "on"
	}
	t.Setenv(common.FreelistPunchEnv, val)
	t.Setenv(common.FreelistPunchReserveEnv, fmt.Sprint(reserve))
	t.Setenv(common.FreelistPunchMaxRangesEnv, "")
	t.Setenv(common.FreelistPunchIntervalEnv, "1h")
}

// newPunchStore makes a WAL file with auto_vacuum NONE, as be-7's World
// file is, opens a store on it, writes punchRows rows and deletes two of
// every three, so the freelist holds several trunks of leaves in many
// ranges. It returns the store, the file and the rows kept.
func newPunchStore(t *testing.T) (*cgo.Store, string, map[int]bool) {
	t.Helper()
	ctx := context.Background()

	// Create the file and its table before the store, which would make a
	// fresh file with incremental auto_vacuum.
	path := filepath.Join(t.TempDir(), "world.db")
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"PRAGMA page_size=4096", "PRAGMA auto_vacuum=NONE", "PRAGMA journal_mode=WAL", "CREATE TABLE kv (key BLOB PRIMARY KEY, value BLOB)"} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	// Open the store and write every row, then delete two of three.
	store, err := cgo.Open(ctx, path, "kv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	putRows(t, store, "k", 0, punchRows)
	kept := map[int]bool{}
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range punchRows {
		if i%3 == 0 {
			kept[i] = true
			continue
		}
		if err := tx.Delete(ctx, punchKey(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return store, path, kept
}

// putRows writes rows from to to under prefix in one transaction.
func putRows(t *testing.T, store *cgo.Store, prefix string, from, to int) {
	t.Helper()
	ctx := context.Background()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := from; i < to; i++ {
		key := append([]byte(prefix), punchKey(i)[1:]...)
		if err := tx.Set(ctx, key, punchValue(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// checkpointEmpty copies the store's whole WAL into the file and empties it.
func checkpointEmpty(t *testing.T, store *cgo.Store) {
	t.Helper()
	var busy, frames, copied int
	if err := store.GetDB().QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &copied); err != nil || busy != 0 {
		t.Fatalf("checkpoint: busy %d, err %v", busy, err)
	}
}

// planFile walks the file's freelist strictly as the punch does.
func planFile(t *testing.T, path string, reserve int64) (sqlitefree.Header, *sqlitefree.Walk, sqlitefree.Plan) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	h, err := sqlitefree.ReadHeader(f, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	w, p, err := sqlitefree.StrictPlan(f, h, reserve)
	if err != nil {
		t.Fatal(err)
	}
	return h, w, p
}

// allocated is the bytes of disk the file holds.
func allocated(t *testing.T, path string) int64 {
	t.Helper()
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		t.Fatal(err)
	}
	return st.Blocks * 512
}

// holes reports, per range, whether the file holds no data blocks there.
func holes(t *testing.T, path string, rs []sqlitefree.Range) []bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := make([]bool, len(rs))
	for i, r := range rs {
		out[i] = sqlitefree.IsHole(f, 4096, r)
	}
	return out
}

// pageRanges is each page as a range of one.
func pageRanges(pages []uint32) []sqlitefree.Range {
	rs := make([]sqlitefree.Range, len(pages))
	for i, p := range pages {
		rs[i] = sqlitefree.Range{Start: p, Count: 1}
	}
	return rs
}

// verifyFile closes nothing: it opens the file on a pool of its own, runs
// integrity_check and reads back every row of want.
func verifyFile(t *testing.T, path string, want map[string][]byte) {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var res string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&res); err != nil || res != "ok" {
		t.Fatalf("integrity_check: %q, %v", res, err)
	}
	for k, v := range want {
		var got []byte
		if err := db.QueryRow("SELECT value FROM kv WHERE key = ?", []byte(k)).Scan(&got); err != nil {
			t.Fatalf("row %s: %v", k, err)
		}
		if !bytes.Equal(got, v) {
			t.Fatalf("row %s changed", k)
		}
	}
}

// keptRows are the rows newPunchStore kept, by key.
func keptRows(kept map[int]bool) map[string][]byte {
	out := map[string][]byte{}
	for i := range kept {
		out[string(punchKey(i))] = punchValue(i)
	}
	return out
}

// TestFreelistPunchOffByDefault proves a store opened without the
// environment variable never punches: a round run as the checkpointer runs it
// does nothing, and no planned range becomes a hole.
func TestFreelistPunchOffByDefault(t *testing.T) {
	// Open a store with a freelist and the variable unset.
	punchEnv(t, false, 1)
	store, path, _ := newPunchStore(t)
	if store.FreelistPunchConfig().Enabled {
		t.Fatal("the punch is on without the environment variable")
	}

	// Run a round and require no punch.
	checkpointEmpty(t, store)
	_, _, p := planFile(t, path, 1)
	before := allocated(t, path)
	common.RunPunchStep(context.Background(), store)
	st := store.Stats()
	if st.PunchRuns+st.PunchSkippedBusy+st.PunchRefused != 0 || st.PunchBytes != 0 {
		t.Fatalf("a round ran: %+v", st)
	}
	for i, hole := range holes(t, path, p.Ranges) {
		if hole {
			t.Fatalf("range %d %+v is a hole", i, p.Ranges[i])
		}
	}
	if after := allocated(t, path); after < before {
		t.Fatalf("allocated %d, before %d", after, before)
	}
}

// TestFreelistPunchMatchesPlan proves a round punches exactly the ranges the
// plan predicts, capped per round and resumed by the next, keeps every
// reserved leaf and trunk allocated, and releases the bytes it counts.
func TestFreelistPunchMatchesPlan(t *testing.T) {
	// Open a store with a freelist and plan its punch.
	punchEnv(t, true, 1)
	store, path, kept := newPunchStore(t)
	checkpointEmpty(t, store)
	h, w, p := planFile(t, path, 1)
	if len(p.Ranges) < 8 || p.KeptPages == 0 || len(w.Trunks) < 3 {
		t.Fatalf("freelist too small for the test: %d ranges, %d kept, %d trunks", len(p.Ranges), p.KeptPages, len(w.Trunks))
	}
	before := allocated(t, path)

	// Punch three ranges and require exactly the first three.
	cfg := store.FreelistPunchConfig()
	cfg.MaxRanges = 3
	store.SetFreelistPunch(cfg)
	common.RunPunchStep(context.Background(), store)
	st := store.Stats()
	var first int64
	for _, r := range p.Ranges[:3] {
		first += int64(r.Count) * h.PageSize
	}
	if st.PunchRuns != 1 || st.PunchRanges != 3 || int64(st.PunchBytes) != first {
		t.Fatalf("capped round: %+v, want 3 ranges of %d bytes", st, first)
	}
	got := holes(t, path, p.Ranges)
	if !got[0] || !got[1] || !got[2] || got[3] {
		t.Fatalf("holes after the capped round: %v", got[:4])
	}

	// Raise the cap and punch the rest.
	cfg.MaxRanges = 1 << 20
	store.SetFreelistPunch(cfg)
	common.RunPunchStep(context.Background(), store)
	st = store.Stats()
	want := int64(p.PunchPages) * h.PageSize
	if st.PunchRuns != 2 || st.PunchRanges != uint64(len(p.Ranges)) || int64(st.PunchBytes) != want {
		t.Fatalf("rounds: %+v, want %d ranges of %d bytes", st, len(p.Ranges), want)
	}
	if freed := before - allocated(t, path); freed != want {
		t.Fatalf("allocation fell %d bytes, plan %d", freed, want)
	}

	// Require every range a hole and every reserved leaf and trunk not.
	for i, hole := range holes(t, path, p.Ranges) {
		if !hole {
			t.Fatalf("range %d %+v not punched", i, p.Ranges[i])
		}
	}
	for i, hole := range holes(t, path, pageRanges(append(append([]uint32{}, w.Trunks...), w.Leaves[:p.KeptPages]...))) {
		if hole {
			t.Fatalf("reserved page %d punched", i)
		}
	}
	if st.PunchHeld <= 0 || st.MaxPunchHold < st.LastPunchHold {
		t.Fatalf("hold times: %+v", st)
	}

	// Require the file intact.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	verifyFile(t, path, keptRows(kept))
}

// TestFreelistPunchSkipsOldSnapshot proves a reader holding a snapshot older
// than the log's end makes the round skip as busy and punch nothing; the
// reader reads its rows intact, and the round after it ends punches.
func TestFreelistPunchSkipsOldSnapshot(t *testing.T) {
	ctx := context.Background()

	// Open a store and a reader whose snapshot holds rows 0, 3, 6...
	punchEnv(t, true, 1)
	store, path, kept := newPunchStore(t)
	rtx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	if _, ok, err := rtx.Get(ctx, punchKey(0)); err != nil || !ok {
		t.Fatal("reader", ok, err)
	}

	// Delete rows the reader still sees, freeing their pages after its
	// snapshot.
	wtx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 900; i += 3 {
		if err := wtx.Delete(ctx, punchKey(i)); err != nil {
			t.Fatal(err)
		}
		delete(kept, i)
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	before := allocated(t, path)

	// Run a round and require a busy skip with nothing punched.
	common.RunPunchStep(ctx, store)
	st := store.Stats()
	if st.PunchSkippedBusy != 1 || st.PunchRuns != 0 || st.PunchBytes != 0 || st.PunchRefused != 0 {
		t.Fatalf("round with an old reader: %+v", st)
	}
	if after := allocated(t, path); after < before {
		t.Fatalf("allocated %d, before %d", after, before)
	}

	// Read the deleted rows through the old snapshot.
	for i := 0; i < 900; i += 3 {
		v, ok, err := rtx.Get(ctx, punchKey(i))
		if err != nil || !ok || !bytes.Equal(v, punchValue(i)) {
			t.Fatalf("reader row %d: %v %v", i, ok, err)
		}
	}

	// End the reader and require the next round to punch.
	rtx.Discard()
	common.RunPunchStep(ctx, store)
	if st := store.Stats(); st.PunchRuns != 1 || st.PunchBytes == 0 {
		t.Fatalf("round after the reader: %+v", st)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	verifyFile(t, path, keptRows(kept))
}

// TestFreelistPunchCheckpointRacesWalk proves no checkpoint can copy frames
// into the file between the walk and the punch. A connection outside the
// store, as another process would, writes rows into the freelist's pages
// and checkpoints them into the file: during the walk the round's write
// lock refuses its write and its read mark blocks the copy; between the
// truncating checkpoint and the lock, the round finds the log no longer
// empty and refuses. With no reserve every free leaf is planned, so a copy
// that landed would be punched away.
func TestFreelistPunchCheckpointRacesWalk(t *testing.T) {
	for _, tc := range []struct {
		name, writeAt string
	}{
		{"write during walk", "walked"},
		{"write before lock", "checkpointed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Open a store punching with no reserve and an outside pool.
			punchEnv(t, true, 0)
			store, path, kept := newPunchStore(t)
			want := keptRows(kept)
			racer, err := sql.Open("sqlite3", path+"?_busy_timeout=100")
			if err != nil {
				t.Fatal(err)
			}
			defer racer.Close()

			// Write at the chosen stage and checkpoint at the walk.
			var writeErr error
			wrote := false
			common.SetPunchHook(store, func(stage string) error {
				if stage == tc.writeAt {
					writeErr = racerWrite(racer, want)
					wrote = writeErr == nil
				}
				if stage == "walked" {
					_, _ = racer.Exec("PRAGMA wal_checkpoint(PASSIVE)")
				}
				return nil
			})
			common.RunPunchStep(context.Background(), store)
			st := store.Stats()

			// Require the write refused during the walk, or the round
			// refused after a write before the lock.
			switch tc.writeAt {
			case "walked":
				if wrote || st.PunchRuns != 1 {
					t.Fatalf("write during the walk landed (%v) or no punch: %+v", writeErr, st)
				}
			case "checkpointed":
				if !wrote || st.PunchRefused != 1 || st.PunchBytes != 0 {
					t.Fatalf("write before the lock (%v), round: %+v", writeErr, st)
				}
			}
			if !wrote {
				for k := range want {
					if strings.HasPrefix(k, "r") {
						delete(want, k)
					}
				}
			}

			// Close every pool and require the file intact.
			_ = racer.Close()
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			verifyFile(t, path, want)
		})
	}
}

// racerWrite writes 200 rows through db in one transaction, adding them to
// want.
func racerWrite(db *sql.DB, want map[string][]byte) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for i := range 200 {
		k := fmt.Sprintf("r%06d", i)
		if _, err := tx.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte(k), punchValue(i+punchRows)); err != nil {
			_ = tx.Rollback()
			return err
		}
		want[k] = punchValue(i + punchRows)
	}
	return tx.Commit()
}

// TestFreelistPunchReuse proves a writer reuses punched pages: after a punch,
// new rows fill the freelist past the reserve into the holes, integrity_check
// is ok and every row reads back.
func TestFreelistPunchReuse(t *testing.T) {
	// Punch a store's freelist.
	punchEnv(t, true, 1)
	store, path, kept := newPunchStore(t)
	checkpointEmpty(t, store)
	h, _, p := planFile(t, path, 1)
	common.RunPunchStep(context.Background(), store)
	if st := store.Stats(); st.PunchRuns != 1 || st.PunchBytes == 0 {
		t.Fatalf("punch: %+v", st)
	}

	// Write more rows than the freelist holds and require some planned
	// range to hold data again.
	putRows(t, store, "n", 0, int(h.FreeCount)+100)
	checkpointEmpty(t, store)
	reused := 0
	for _, hole := range holes(t, path, p.Ranges) {
		if !hole {
			reused++
		}
	}
	var free int
	if err := store.GetDB().QueryRow("PRAGMA freelist_count").Scan(&free); err != nil {
		t.Fatal(err)
	}
	if reused == 0 || uint64(free) >= uint64(h.FreeCount)-p.KeptPages {
		t.Fatalf("punched pages not reused: %d ranges hold data, freelist %d of %d", reused, free, h.FreeCount)
	}

	// Require the file intact with every old and new row.
	want := keptRows(kept)
	for i := range int(h.FreeCount) + 100 {
		want["n"+string(punchKey(i)[1:])] = punchValue(i)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	verifyFile(t, path, want)
}

// errStopAfterPunch ends a round after the punch, before its fsync.
var errStopAfterPunch = errors.New("stop after punch")

// TestFreelistPunchCrashBeforeFsync proves a round stopped after the punch,
// before the fsync, leaves a file world-punch -crosscheck -scan passes: the
// file is copied as it stands after the punch, with the WAL empty and the
// write lock held. WORLD_PUNCH_BIN names the binary; without it the test
// builds this tree's.
func TestFreelistPunchCrashBeforeFsync(t *testing.T) {
	// Copy the file at the punch and stop the round there.
	punchEnv(t, true, 1)
	store, path, kept := newPunchStore(t)
	checkpointEmpty(t, store)
	_, _, p := planFile(t, path, 1)
	crash := filepath.Join(t.TempDir(), "crash.db")
	common.SetPunchHook(store, func(stage string) error {
		if stage != "punched" {
			return nil
		}
		return errors.Join(copyFile(path, crash), errStopAfterPunch)
	})
	common.RunPunchStep(context.Background(), store)
	if st := store.Stats(); st.PunchRefused != 1 || st.PunchRuns != 0 {
		t.Fatalf("stopped round: %+v", st)
	}

	// Require the copy to hold the punch: every planned range reads zero.
	data, err := os.ReadFile(crash)
	if err != nil {
		t.Fatal(err)
	}
	zero := make([]byte, 4096)
	for _, r := range p.Ranges {
		for pg := r.Start; pg < r.Start+r.Count; pg++ {
			if !bytes.Equal(data[int64(pg-1)*4096:int64(pg)*4096], zero) {
				t.Fatalf("page %d of the copy is not zero", pg)
			}
		}
	}

	// Run the crosscheck scan on the copy and require PASS.
	bin := os.Getenv("WORLD_PUNCH_BIN")
	if bin == "" {
		bin = filepath.Join(t.TempDir(), "world-punch")
		if out, err := exec.Command("go", "build", "-o", bin, "github.com/s4wave/spacewave/cmd/world-punch").CombinedOutput(); err != nil {
			t.Fatalf("build world-punch: %v\n%s", err, out)
		}
	}
	out, err := exec.Command(bin, "-crosscheck", "-scan", crash).CombinedOutput()
	t.Logf("world-punch -crosscheck -scan:\n%s", out)
	if err != nil || !strings.Contains(string(out), "result: PASS") {
		t.Fatalf("scan of the stopped file: %v", err)
	}

	// Require the copy and the store's file intact.
	verifyFile(t, crash, keptRows(kept))
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	verifyFile(t, path, keptRows(kept))
}

// copyFile copies src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}
