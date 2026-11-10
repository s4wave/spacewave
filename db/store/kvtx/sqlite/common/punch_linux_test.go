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
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/sqlitefree"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/cgo"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
	"golang.org/x/sys/unix"
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
// test runs rounds and no disk floor past the least, four reserves.
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
	t.Setenv(common.FreelistPunchFloorEnv, "0")
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

// TestFreelistPunchLookBudget proves a round over a file already punched
// through looks at no more than four times its cap of ranges, so a round
// after the cursor wraps costs no more than any other, and that the next
// round goes on from the last range it looked at.
func TestFreelistPunchLookBudget(t *testing.T) {
	// Punch the whole plan, which brings the cursor back to the start.
	punchEnv(t, true, 0)
	store, path, _ := newPunchStore(t)
	checkpointEmpty(t, store)
	_, _, p := planFile(t, path, 0)
	if len(p.Ranges) < 12 {
		t.Fatalf("freelist too small for the test: %d ranges", len(p.Ranges))
	}
	common.RunPunchStep(context.Background(), store)
	if st := store.Stats(); st.PunchRanges != uint64(len(p.Ranges)) || common.PunchCursor(store) != 0 {
		t.Fatalf("whole punch: %+v, cursor %d", st, common.PunchCursor(store))
	}

	// Cap rounds at one range and require each to look at four holes.
	cfg := store.FreelistPunchConfig()
	cfg.MaxRanges = 1
	store.SetFreelistPunch(cfg)
	for i := 1; i <= 2; i++ {
		common.RunPunchStep(context.Background(), store)
		end := p.Ranges[4*i-1].Start + p.Ranges[4*i-1].Count
		if st := store.Stats(); st.PunchRuns != uint64(1+i) || st.PunchRanges != uint64(len(p.Ranges)) || common.PunchCursor(store) != end {
			t.Fatalf("round %d over holes: %+v, cursor %d, want %d", i, st, common.PunchCursor(store), end)
		}
	}
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
// and checkpoints them into the file: during the walk the write lands in
// the log and the round's read mark blocks the copy; between the truncating
// checkpoint and the read mark, the round finds the log no longer empty and
// skips as busy. With no reserve every free leaf is planned, so a copy that
// landed would be punched away.
func TestFreelistPunchCheckpointRacesWalk(t *testing.T) {
	for _, tc := range []struct {
		name, writeAt string
	}{
		{"write during walk", "walked"},
		{"write before read", "checkpointed"},
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

			// Require the punch with the write landed during the walk, or
			// the round skipped after a write before the read mark.
			switch tc.writeAt {
			case "walked":
				if !wrote || st.PunchRuns != 1 {
					t.Fatalf("write during the walk (%v), round: %+v", writeErr, st)
				}
			case "checkpointed":
				if !wrote || st.PunchSkippedBusy != 1 || st.PunchRefused != 0 || st.PunchBytes != 0 {
					t.Fatalf("write before the read mark (%v), round: %+v", writeErr, st)
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

// TestFreelistPunchReuseBeforePunch proves the plan a round walked is still
// right when it punches, though writers reuse its pages in between. Between
// the walk and the punch, a connection outside the store commits rows into
// free pages the plan holds (X), a reader pins a snapshot after X, a second
// commit writes page 1 again (Y), and a checkpoint runs. Were the file
// writable then, the checkpoint would copy X's pages in but not page 1,
// whose newest frame is past the reader, so the header would read as at the
// walk while planned pages held live rows. The round's read mark, held from
// before the walk to after the fsync, keeps the checkpoint from copying
// anything; the rows of X read back intact after the round and a full
// checkpoint over the punched file.
func TestFreelistPunchReuseBeforePunch(t *testing.T) {
	ctx := context.Background()

	// Open a store punching every free leaf, a writer outside it that never
	// checkpoints by itself, and an outside reader.
	punchEnv(t, true, 0)
	store, path, kept := newPunchStore(t)
	want := keptRows(kept)
	checkpointEmpty(t, store)
	racer, err := sql.Open("sqlite3", path+"?_busy_timeout=100")
	if err != nil {
		t.Fatal(err)
	}
	defer racer.Close()
	racer.SetMaxOpenConns(1)
	if _, err := racer.Exec("PRAGMA wal_autocheckpoint=0"); err != nil {
		t.Fatal(err)
	}
	reader, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	h0, _, plan0 := planFile(t, path, 0)

	// Between the walk and the punch, commit X, pin a reader after it,
	// commit Y and checkpoint.
	var stepErr error
	var busy, frames, copied int
	common.SetPunchHook(store, func(stage string) error {
		if stage != "walked" {
			return nil
		}
		stepErr = func() error {
			if err := racerWrite(racer, want); err != nil {
				return fmt.Errorf("commit X: %w", err)
			}
			rtx, err := reader.Begin()
			if err != nil {
				return err
			}
			defer rtx.Rollback() //nolint:errcheck
			var n int
			if err := rtx.QueryRow("SELECT count(*) FROM kv").Scan(&n); err != nil {
				return fmt.Errorf("reader: %w", err)
			}
			want["y"] = punchValue(9999)
			if _, err := racer.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte("y"), want["y"]); err != nil {
				return fmt.Errorf("commit Y: %w", err)
			}
			return racer.QueryRow("PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &frames, &copied)
		}()
		return nil
	})
	common.RunPunchStep(ctx, store)
	if stepErr != nil {
		t.Fatal(stepErr)
	}

	// Require the punch of the whole plan, with the checkpoint copying
	// nothing while the round held its read mark.
	st := store.Stats()
	if st.PunchRuns != 1 || st.PunchRefused != 0 || int64(st.PunchBytes) != int64(plan0.PunchPages)*h0.PageSize {
		t.Fatalf("round: %+v, plan of %d pages", st, plan0.PunchPages)
	}
	if frames == 0 || copied != 0 {
		t.Fatalf("checkpoint during the round: busy %d, frames %d, copied %d", busy, frames, copied)
	}

	// Checkpoint every frame into the punched file and read every row back.
	if err := racer.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &copied); err != nil || busy != 0 || frames != copied {
		t.Fatalf("checkpoint after the round: busy %d, frames %d, copied %d, %v", busy, frames, copied, err)
	}
	_ = reader.Close()
	_ = racer.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	verifyFile(t, path, want)
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

// TestFreelistPunchDiskFloor proves a round punches only while the disk's
// free space less the file's holes is at least the floor: one byte under
// skips it with nothing punched, and the holes a round made count against
// the free space of the next.
func TestFreelistPunchDiskFloor(t *testing.T) {
	ctx := context.Background()
	const floor = 1 << 40

	// Open a store punching three ranges a round over a disk whose free
	// space the test sets.
	punchEnv(t, true, 1)
	store, path, kept := newPunchStore(t)
	cfg := store.FreelistPunchConfig()
	cfg.MaxRanges, cfg.Floor = 3, floor
	store.SetFreelistPunch(cfg)
	var free int64
	defer common.SetPunchDiskFree(func(string) (int64, error) { return free, nil })()
	checkpointEmpty(t, store)
	holes := func() int64 {
		var st syscall.Stat_t
		if err := syscall.Stat(path, &st); err != nil {
			t.Fatal(err)
		}
		return max(st.Size-st.Blocks*512, 0)
	}

	// One byte under the floor: the round is skipped and punches nothing.
	free = floor + holes() - 1
	before := allocated(t, path)
	common.RunPunchStep(ctx, store)
	if st := store.Stats(); st.PunchSkippedFloor != 1 || st.PunchRuns != 0 || st.PunchBytes != 0 || allocated(t, path) != before {
		t.Fatalf("round under the floor: %+v", st)
	}

	// At the floor the round punches.
	free = floor + holes()
	common.RunPunchStep(ctx, store)
	st := store.Stats()
	if st.PunchRuns != 1 || st.PunchBytes == 0 {
		t.Fatalf("round at the floor: %+v", st)
	}

	// The holes it made count: the same free space is now under the floor.
	made := holes()
	if made < int64(st.PunchBytes) {
		t.Fatalf("holes %d, punched %d", made, st.PunchBytes)
	}
	common.RunPunchStep(ctx, store)
	if st := store.Stats(); st.PunchSkippedFloor != 2 || st.PunchRuns != 1 {
		t.Fatalf("round with the same free space after holes: %+v", st)
	}
	free = floor + made
	common.RunPunchStep(ctx, store)
	if st := store.Stats(); st.PunchRuns != 2 {
		t.Fatalf("round with free space for the holes: %+v", st)
	}

	// A floor under four reserves is raised to that.
	store.SetFreelistPunch(common.FreelistPunch{Enabled: true, Reserve: 1 << 30, MaxRanges: 3})
	if got := store.FreelistPunchConfig().Floor; got != 4<<30 {
		t.Fatalf("floor %d with a 1 GiB reserve", got)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	verifyFile(t, path, keptRows(kept))
}

// TestFreelistPunchHoldLarge measures rounds on the file named by
// SPACEWAVE_PUNCH_LARGE_DB, such as one the size of be-7's: the time of each
// stage, cold (the file evicted from the page cache first) and warm. It is
// skipped without the variable, and it punches the file it is given.
func TestFreelistPunchHoldLarge(t *testing.T) {
	path := os.Getenv("SPACEWAVE_PUNCH_LARGE_DB")
	if path == "" {
		t.Skip("SPACEWAVE_PUNCH_LARGE_DB is not set")
	}
	ctx := context.Background()

	// Open the store with the punch on and only the test running rounds.
	punchEnv(t, true, 1<<30)
	store, err := cgo.Open(ctx, path, "kv")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Note when each stage of a round is reached.
	var marks []time.Time
	common.SetPunchHook(store, func(string) error {
		marks = append(marks, time.Now())
		return nil
	})

	// With a WAL size set, measure the round on a WAL that large with a
	// writer committing through it, against a bare truncating checkpoint.
	if mb, _ := strconv.Atoi(os.Getenv("SPACEWAVE_PUNCH_LARGE_WAL_MB")); mb > 0 {
		largeWALRounds(t, store, path, int64(mb)<<20, &marks)
		return
	}

	// Run cold rounds, then warm ones, logging the stages of each.
	for i, cold := range []bool{true, false, false} {
		if cold {
			evictFile(t, path)
		}
		marks = marks[:0]
		start := time.Now()
		common.RunPunchStep(ctx, store)
		end := time.Now()
		st := store.Stats()
		var stages []string
		prev := start
		for _, m := range append(marks, end) {
			stages = append(stages, m.Sub(prev).String())
			prev = m
		}
		t.Logf("round %d cold=%v: total %v, write lock %v, read mark %v, stages (to flushed, checkpointed, walked, punched, end) %v; runs %d busy %d refused %d ranges %d bytes %d",
			i, cold, end.Sub(start), st.LastPunchHold, st.LastPunchSnapshot, stages, st.PunchRuns, st.PunchSkippedBusy, st.PunchRefused, st.PunchRanges, st.PunchBytes)
	}
}

// largeWALRounds fills the WAL of the store's file to walBytes through a
// connection outside the store, then, with a writer committing a small row
// every 5ms, runs a punch round, and on a WAL filled again a bare TRUNCATE
// checkpoint as a round without the passive copy first would hold; it logs
// the stages, the write lock hold and the writer's slowest commit of each.
func largeWALRounds(t *testing.T, store *cgo.Store, path string, walBytes int64, marks *[]time.Time) {
	ctx := context.Background()

	// Open one outside connection that never checkpoints by itself.
	raw, err := sql.Open("sqlite3", path+"?_busy_timeout=10000")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	raw.SetMaxOpenConns(1)
	if _, err := raw.Exec("PRAGMA wal_autocheckpoint=0"); err != nil {
		t.Fatal(err)
	}

	// fill writes 16 MiB transactions of 64 KiB rows until the WAL holds
	// walBytes.
	n := 0
	fill := func() {
		start := time.Now()
		for {
			info, err := os.Stat(path + "-wal")
			if err == nil && info.Size() >= walBytes {
				t.Logf("WAL filled to %d bytes in %v", info.Size(), time.Since(start))
				return
			}
			tx, err := raw.Begin()
			if err != nil {
				t.Fatal(err)
			}
			for range 256 {
				n++
				if _, err := tx.Exec("INSERT OR REPLACE INTO kv VALUES (?, randomblob(65536))", fmt.Appendf(nil, "w%09d", n)); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
		}
	}

	// writer commits a small row every 5ms until stop closes, sending its
	// slowest commit and counting the commits that failed.
	failed := 0
	writer := func(stop chan struct{}, slowest chan time.Duration) {
		var worst time.Duration
		for i := 0; ; i++ {
			select {
			case <-stop:
				slowest <- worst
				return
			case <-time.After(5 * time.Millisecond):
			}
			start := time.Now()
			if _, err := raw.Exec("INSERT OR REPLACE INTO kv VALUES (?, randomblob(512))", fmt.Appendf(nil, "s%09d", i)); err != nil {
				failed++
			}
			worst = max(worst, time.Since(start))
		}
	}

	// Run a round on the full WAL with the writer going.
	fill()
	stop, slowest := make(chan struct{}), make(chan time.Duration)
	go writer(stop, slowest)
	*marks = (*marks)[:0]
	start := time.Now()
	common.RunPunchStep(ctx, store)
	end := time.Now()
	close(stop)
	worst := <-slowest
	st := store.Stats()
	var stages []string
	prev := start
	for _, m := range append(*marks, end) {
		stages = append(stages, m.Sub(prev).String())
		prev = m
	}
	t.Logf("round on the WAL: total %v, write lock held %v, read mark held %v, stages (to flushed, checkpointed, walked, punched, end) %v, writer's slowest commit %v, failed %d; runs %d busy %d refused %d",
		end.Sub(start), st.LastPunchHold, st.LastPunchSnapshot, stages, worst, failed, st.PunchRuns, st.PunchSkippedBusy, st.PunchRefused)

	// Time a bare truncating checkpoint on a WAL filled again, with the
	// writer going, through a store connection.
	fill()
	failed = 0
	stop = make(chan struct{})
	go writer(stop, slowest)
	var busy, frames, copied int
	start = time.Now()
	err = store.GetDB().QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &copied)
	took := time.Since(start)
	close(stop)
	worst = <-slowest
	t.Logf("bare TRUNCATE on the WAL: %v (busy %d, frames %d, copied %d, err %v), writer's slowest commit %v, failed %d", took, busy, frames, copied, err, worst, failed)
}

// evictFile drops the clean pages of path from the page cache.
func evictFile(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED); err != nil {
		t.Fatal(err)
	}
}
