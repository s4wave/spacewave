package common_test

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// restartBound is the write-ahead log bound of the gap tests: small, so a
// writer passes it within a checkpoint interval.
const restartBound = 256 << 10

// restartKeys is how many keys the gap tests' writers cycle through, so the
// database file stays near 16 MiB however long they write.
const restartKeys = 4096

// restartWALCap is the log size past which the gap tests' writers fail. Only
// a runaway log reaches restartWALCap. The writers also fail once they have
// written restartByteCap or after restartTimeCap, so a step that never comes
// cannot fill the disk.
const (
	restartWALCap  = 256 << 20
	restartByteCap = 1 << 30
	restartTimeCap = 60 * time.Second
)

// TestWALGapStopped stops the store at each step of fillWALGap, killing its
// process or closing it, reopens it and checks that every commit made before
// the stop reads back. A killed process leaves its writes in the page cache,
// so the kill cases show that every commit is in the log or the database file
// at the step, not that either reached the device.
func TestWALGapStopped(t *testing.T) {
	// A child process writes until the step it was given, then waits to be
	// killed.
	if env := os.Getenv("SPACEWAVE_WAL_RESTART_CHILD"); env != "" {
		path, step, _ := strings.Cut(env, ",")
		walRestartChild(t, path, step)
		return
	}

	// Kill the process at a pass, after a gap its deadline interrupted, and
	// while it holds the writers back.
	for _, step := range []string{"pass", "gap false", "hold"} {
		t.Run("kill "+step, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kill.db")
			cmd := exec.Command(os.Args[0], "-test.run=^TestWALGapStopped$", "-test.v")
			cmd.Env = append(os.Environ(), "SPACEWAVE_WAL_RESTART_CHILD="+path+","+step)
			out, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			stuck := time.AfterFunc(90*time.Second, func() { _ = cmd.Process.Kill() })
			defer stuck.Stop()
			committed, capped := -1, ""
			for sc := bufio.NewScanner(out); committed < 0 && capped == "" && sc.Scan(); {
				if n, ok := strings.CutPrefix(sc.Text(), "committed "); ok {
					committed, _ = strconv.Atoi(n)
				}
				if c, ok := strings.CutPrefix(sc.Text(), "capped: "); ok {
					capped = c
				}
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			if capped != "" {
				t.Fatalf("child stopped before %q: %s", step, capped)
			}
			if committed < 0 {
				t.Fatalf("child exited or stalled before %q", step)
			}
			checkCommitted(t, path, committed)
		})
	}

	// Close the store at a pass.
	t.Run("close pass", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "close.db")
		passed := make(chan struct{}, 1)
		common.SetCheckpointHook(func(step string) {
			if step == "pass" {
				select {
				case passed <- struct{}{}:
					time.Sleep(50 * time.Millisecond)
				default:
				}
			}
		}, common.DefaultGapHold)
		defer common.SetCheckpointHook(nil, common.DefaultGapHold)
		db := openRestartStore(t, path)
		var committed atomic.Int64
		writing, stopWriting := context.WithCancel(context.Background())
		defer stopWriting()
		writeDone := make(chan struct{})
		go func() {
			defer close(writeDone)
			if err := writeUntilClosed(writing, db, path, &committed); err != nil {
				t.Error(err)
			}
		}()

		// Stop the writer, then close the store while the pass runs.
		<-passed
		stopWriting()
		<-writeDone
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		checkCommitted(t, path, int(committed.Load()))
	})
}

// TestWALGapPinned holds one read transaction open while a writer commits
// 128 MiB past the log's bound. The reader keeps SQLite from copying past its
// snapshot or resetting the log, so holding the writer back cannot shrink the
// log, and the checkpointer must not: at most the passes before it finds the
// log pinned may hold the writer. Measuring, no commit waits long for the
// turn.
func TestWALGapPinned(t *testing.T) {
	h := timeHolds(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pinned.db")
	db := openRestartStore(t, path)
	defer db.Close()

	// commit writes 16 rows of 4 KiB as one ordered commit and returns how
	// long it waited for the writer turn.
	value := make([]byte, 4096)
	commit := func(i int) time.Duration {
		began := time.Now()
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		turn := time.Since(began)
		for j := range 16 {
			if err := tx.Set(ctx, restartKey(i*16+j), value); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(kvtx.WithOrderedCommitContext(ctx)); err != nil {
			t.Fatal(err)
		}
		return turn
	}

	// Pin the log with a reader whose snapshot holds the first commit.
	commit(0)
	read, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	if _, _, err := read.Get(ctx, restartKey(0)); err != nil {
		t.Fatal(err)
	}

	// Commit 128 MiB, or for restartTimeCap, timing each wait for the turn.
	var turn time.Duration
	var peak int64
	start := time.Now()
	n := 1
	for ; n < 2048 && time.Since(start) < restartTimeCap; n++ {
		turn = max(turn, commit(n))
		if info, err := os.Stat(path + "-wal"); err == nil {
			peak = max(peak, info.Size())
		}
	}
	took := time.Since(start)

	// The pin kept the log past the hold multiple, and the writer ran free.
	t.Logf("%d commits in %v, %.0f MiB/s, longest turn wait %v, %s, log peak %.0fx its bound", n-1, took.Round(time.Millisecond), float64(n-1)*16*4096/(1<<20)/took.Seconds(), turn, h, float64(peak)/restartBound)
	if peak < 8*restartBound {
		t.Errorf("log peaked at %.1fx its bound; the reader did not pin it past 8x", float64(peak)/restartBound)
	}
	if n := h.holds.Load(); n > 2 {
		t.Errorf("%d passes held the writer back under a pinned log, want at most 2", n)
	}
	h.checkTurn(t, turn)
}

// TestWALGapHold checks that a log grown by ordered commits resets after the
// writer and any concurrent reader stop. A reader may pin the log, and the
// scheduler and device set the copy rate, so peak size is diagnostic.
// TestWALGapCopyRegimes checks recovery and waits with delayed copies.
func TestWALGapHold(t *testing.T) {
	// Exercise checkpoint recovery both with and without concurrent reads.
	for _, read := range []bool{true, false} {
		t.Run("read "+strconv.FormatBool(read), func(t *testing.T) {
			// Open a store whose log is bounded at 16 MiB.
			h := timeHolds(t)
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "hold.db")
			const bound = 16 << 20
			db, err := store_kvtx_sqlite.OpenWithPragmas(ctx, path, "kv", store_kvtx_sqlite.Pragmas{WALCheckpointBytes: bound})
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			// Read constantly while the writes run, as lookups do.
			reading, stopReading := context.WithCancel(ctx)
			readDone := make(chan struct{})
			defer func() {
				stopReading()
				<-readDone
			}()
			go func() {
				defer close(readDone)
				for i := 0; read; i++ {
					select {
					case <-reading.Done():
						return
					default:
					}
					if tx, err := db.NewTransaction(ctx, false); err == nil {
						_, _, _ = tx.Get(ctx, []byte("key-"+strconv.Itoa(i%20000)))
						tx.Discard()
					}
				}
			}()

			// Write 256 MiB in 4096 ordered commits over 20,000 keys.
			// Count the work independently of the host's scheduling rate.
			value := make([]byte, 4096)
			var longest, turn time.Duration
			var peak int64
			rows := 0
			start := time.Now()
			for range 4096 {
				began := time.Now()
				tx, err := db.NewTransaction(ctx, true)
				if err != nil {
					t.Fatal(err)
				}
				turn = max(turn, time.Since(began))
				for range 16 {
					if err := tx.Set(ctx, []byte("key-"+strconv.Itoa(rows%20000)), value); err != nil {
						t.Fatal(err)
					}
					rows++
				}
				if err := tx.Commit(kvtx.WithOrderedCommitContext(ctx)); err != nil {
					t.Fatal(err)
				}
				longest = max(longest, time.Since(began))
				if info, err := os.Stat(path + "-wal"); err == nil {
					peak = max(peak, info.Size())
				}
			}
			took := time.Since(start)
			stopReading()
			<-readDone

			// Report throughput and peak size without assuming a copy rate or
			// a bound on how long a concurrent reader pins its snapshot.
			t.Logf("%d commits in %v, %.0f MiB/s, longest turn wait %v, longest commit %v, %s, log peak %.2fx its bound", rows/16, took.Round(time.Millisecond), float64(rows)*4096/(1<<20)/took.Seconds(), turn, longest, h, float64(peak)/bound)
			h.checkTurn(t, turn)
			h.checkReset(t, db, path, bound)
		})
	}
}

// TestWALGapLargeCommits commits 8 MiB at a time, 2048 frames each, back to
// back past a 256 KiB log bound, beside a writer of 16-frame commits. No hold
// of the turn can copy such a commit within its slice, so without the writer
// waiting for its own frames the log grows without bound. With that wait, on a
// disk that copies what the writers add during a pass within walFlowWaitMax,
// the log stays within 8 times its bound plus one large commit in flight and
// what the small writer commits while the holds run, and a reader that pins the
// log ends the wait. The real disk may be slower than that, so this run asserts
// the log's bound only when measuring. TestWALGapCopyRegimes checks recovery
// and wait budgets with delayed copies.
func TestWALGapLargeCommits(t *testing.T) {
	for _, pin := range []bool{false, true} {
		t.Run("pin "+strconv.FormatBool(pin), func(t *testing.T) {
			r := runLargeCommits(t, pin)
			if pin {
				if r.peak < 8*restartBound {
					t.Errorf("log peaked at %.1fx its bound; the reader did not pin it past 8x", float64(r.peak)/restartBound)
				}
				if r.longest > 2*common.CheckpointInterval {
					t.Errorf("a large commit under a pinned log took %v, want at most %v", r.longest, 2*common.CheckpointInterval)
				}
				return
			}
			if measuring {
				r.checkBound(t)
				r.checkSmall(t)
			}

			// Commit alone, as a scan's last batches do: no other writer
			// begins on the copied log to reset it, so each wait must end
			// once a pass copies the log, not on the reset.
			ctx := context.Background()
			alone, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			for i := range 4 {
				took := r.commit(alone, i*2048, 2048)
				if alone.Err() != nil {
					t.Fatalf("lone large commit %d waited %v, until its deadline", i, took)
				}
			}
		})
	}
}

// TestWALGapCopyRegimes runs TestWALGapLargeCommits' writers with the device
// out of the copy and a fixed delay added to each pass. The actual pass also
// copies through the page cache, so its measured duration supplies the copy
// budget. With passes inside walFlowWaitMax the gap closes once writers stop
// and the next writer resets the log. With passes beyond it every gap pass
// ends at its deadline and the log may grow, but no small commit waits past
// one gap hold and one slice, and no large commit's wait passes the copy of
// its frames or walFlowWaitMax, whichever is longer.
func TestWALGapCopyRegimes(t *testing.T) {
	for _, delay := range []time.Duration{100 * time.Millisecond, 400 * time.Millisecond} {
		t.Run("copy "+delay.String(), func(t *testing.T) {
			// Restore after the store closes, which runLargeCommits defers.
			t.Cleanup(common.SetDiskless(delay))
			r := runLargeCommits(t, false)
			r.checkSmall(t)
			if delay <= common.FlowWaitMax {
				r.checkReset()
				return
			}

			// Budget the actual copy of the in-flight pass and the next,
			// with a gap deadline and one slice between. The injected delay
			// excludes SQLite's copy, which depends on host throughput.
			copyTime := time.Duration(r.holds.copyLongest.Load())
			if copyTime < delay {
				t.Fatalf("no complete checkpoint measured: longest %v, delay %v", copyTime, delay)
			}
			if limit := 2*copyTime + common.FlowWaitMax + common.TurnHold + waitSlack; r.st.FlowWaitMax > limit {
				t.Errorf("a large commit waited %v in the flow, want at most %v", r.st.FlowWaitMax, limit)
			}
		})
	}
}

// waitSlack is what the regime checks allow past their measured work and
// background hold budgets for scheduler delays.
const waitSlack = 100 * time.Millisecond

// largeRun is what runLargeCommits measured.
type largeRun struct {
	// commit commits rows rows of 4 KiB from key first as one ordered
	// commit under ctx, returning how long it took from the wait for the
	// writer turn to Commit's return.
	commit func(ctx context.Context, first, rows int) time.Duration
	// checkReset requires checkpoint recovery after the writers have stopped.
	checkReset func()
	// n counts large commits.
	n int
	// took is the time taken by the large writer.
	took time.Duration
	// longest is the large writer's longest commit.
	longest time.Duration
	// small is the small writer's longest commit.
	small time.Duration
	// largeHold is the longest foreground hold by the other writer.
	largeHold atomic.Int64
	// smallHold is the longest foreground hold by the small writer itself.
	smallHold atomic.Int64
	// holds measures checkpointer holds and actual pass durations.
	holds *holdTimer
	// peak is the largest log a commit left.
	peak int64
	// st is the store's counters after the run.
	st common.StoreStats
}

// largeCommit is the log bytes of one 8 MiB commit.
const largeCommit = 2048 * (4096 + 24)

// runLargeCommits commits 40 large commits beside a small writer, pinning
// the log with a reader first when pin is set, and returns the open store's
// measurements.
func runLargeCommits(t *testing.T, pin bool) *largeRun {
	// Observe checkpoint holds before opening the measured store.
	r := &largeRun{holds: timeHolds(t)}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "large.db")
	db := openRestartStore(t, path)
	t.Cleanup(func() { _ = db.Close() })
	r.checkReset = func() { r.holds.checkReset(t, db, path, restartBound) }

	// commit as largeRun describes.
	value := make([]byte, 4096)
	r.commit = func(ctx context.Context, first, rows int) time.Duration {
		// Take the writer turn while timing the complete commit.
		began := time.Now()
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Error(err)
			return 0
		}

		// Measure foreground work separately from checkpoint interference.
		tx.(*common.Tx).ObserveWriteHold(func(held time.Duration) {
			target := &r.largeHold
			if rows == 16 {
				target = &r.smallHold
			}
			for old := target.Load(); int64(held) > old && !target.CompareAndSwap(old, int64(held)); old = target.Load() {
			}
		})

		// Write this transaction's rows before committing.
		for j := range rows {
			if err := tx.Set(ctx, restartKey((first+j)%restartKeys), value); err != nil {
				t.Error(err)
				tx.Discard()
				return 0
			}
		}
		if err := tx.Commit(kvtx.WithOrderedCommitContext(ctx)); err != nil {
			t.Error(err)
		}
		return time.Since(began)
	}

	// Pin the log with a reader whose snapshot holds the first commit.
	r.commit(ctx, 0, 16)
	if pin {
		read, err := db.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(read.Discard)
		if _, _, err := read.Get(ctx, restartKey(0)); err != nil {
			t.Fatal(err)
		}
	}

	// Run the small writer beside the large one, timing its commits.
	stop := make(chan struct{})
	smallDone := make(chan time.Duration)
	go func() {
		var longest time.Duration
		for i := 0; ; i++ {
			select {
			case <-stop:
				smallDone <- longest
				return
			default:
			}
			longest = max(longest, r.commit(ctx, i*16, 16))
		}
	}()

	// Commit 40 large commits, 320 MiB, or for restartTimeCap, timing each
	// and the log after it.
	start := time.Now()
	for ; r.n < 40 && time.Since(start) < restartTimeCap; r.n++ {
		r.longest = max(r.longest, r.commit(ctx, r.n*2048, 2048))
		if info, err := os.Stat(path + "-wal"); err == nil {
			r.peak = max(r.peak, info.Size())
		}
	}
	r.took = time.Since(start)
	close(stop)
	r.small = <-smallDone

	// Read the largest log a commit left, which a commit that waited no
	// longer sees once Commit returns.
	common.EachStoreStats(func(s common.StoreStats) {
		if filepath.Base(s.File) == filepath.Base(path) {
			r.st = s
		}
	})
	r.peak = max(r.peak, r.st.LogPeakBytes)
	t.Logf("%d large commits in %v, longest %v, %d waits, longest %v, total %v, small writer's longest commit %v, log peak %.1fx its bound", r.n, r.took.Round(time.Millisecond), r.longest, r.st.FlowWaits, r.st.FlowWaitMax, r.st.FlowWait, r.small, float64(r.peak)/restartBound)
	t.Logf("timing components: longest copy %v, background hold %v, foreground large hold %v, foreground small hold %v", time.Duration(r.holds.copyLongest.Load()), time.Duration(r.holds.longest.Load()), time.Duration(r.largeHold.Load()), time.Duration(r.smallHold.Load()))
	return r
}

// checkBound checks that the log stayed within 8 bounds, the large commit in
// flight, and as much again for the holds' overshoot.
func (r *largeRun) checkBound(t *testing.T) {
	if limit := int64(8*restartBound + 2*largeCommit); r.peak > limit {
		t.Errorf("log peaked at %d bytes, %.1fx its bound, want at most %d: 8 bounds, the large commit in flight, and as much again for the holds' overshoot", r.peak, float64(r.peak)/restartBound, limit)
	}
}

// checkSmall checks the background hold cap independently, then budgets a
// small commit's own work and the one foreground transaction ahead of it.
func (r *largeRun) checkSmall(t *testing.T) {
	// Keep the background cap independent of foreground work and copy speed.
	background := common.FlowWaitMax + common.TurnHold + waitSlack
	if held := time.Duration(r.holds.longest.Load()); held > background {
		t.Errorf("checkpointer held the writer for %v, want at most %v", held, background)
	}

	// A FIFO writer turn queues behind one transaction from the large writer.
	foreground := time.Duration(r.largeHold.Load() + r.smallHold.Load())
	if limit := background + foreground; r.small > limit {
		t.Errorf("small writer's longest commit took %v, want at most %v (%v background and scheduler, %v foreground work)", r.small, limit, background, foreground)
	}
}

// measuring is whether the tests assert wall-clock bounds, set by
// SPACEWAVE_SQLITE_MEASURE for a run alone on its host. With one writer, its
// wait for the turn is the rest of a checkpointer hold, and a hold waits on
// the device and the scheduler, so beside other jobs either passes any bound
// tight enough to tell a 25 ms slice from the restart it replaced.
var measuring = os.Getenv("SPACEWAVE_SQLITE_MEASURE") != ""

// holdTimer counts the passes that hold the writers back and times each hold
// of the writer turn, from the checkpointer's hook. The test reads it while
// the checkpointer may still write it, hence the atomics.
type holdTimer struct {
	// passes identifies the current gap-filling pass.
	passes atomic.Uint64
	// gapPass identifies the latest completed gap, even if notices coalesce.
	gapPass atomic.Uint64
	// gaps reports completed gap passes without blocking the checkpointer.
	gaps chan struct{}
	// holds counts passes that held the writer back.
	holds atomic.Int32
	// taken is when the checkpointer took its current writer turn.
	taken atomic.Int64
	// longest is the longest checkpointer hold of the writer turn.
	longest atomic.Int64
	// copyTaken is when the current off-turn pass began.
	copyTaken atomic.Int64
	// copyLongest is the longest completed off-turn pass, including its copy.
	copyLongest atomic.Int64
}

// timeHolds sets the checkpointer's hook to a new holdTimer for the rest of
// the test.
func timeHolds(t *testing.T) *holdTimer {
	h := &holdTimer{gaps: make(chan struct{}, 1)}
	common.SetCheckpointHook(func(step string) {
		switch step {
		case "pass":
			h.passes.Add(1)
		case "gap true":
			h.gapPass.Store(h.passes.Load())
			select {
			case h.gaps <- struct{}{}:
			default:
			}
		case "copy":
			h.copyTaken.Store(time.Now().UnixNano())
		case "written back":
			h.copyLongest.Store(max(h.copyLongest.Load(), time.Now().UnixNano()-h.copyTaken.Load()))
		case "hold":
			h.holds.Add(1)
		case "turn":
			h.taken.Store(time.Now().UnixNano())
		case "free":
			// Only the checkpointer takes and frees the turn, one hold
			// at a time.
			h.longest.Store(max(h.longest.Load(), time.Now().UnixNano()-h.taken.Load()))
		}
	}, common.DefaultGapHold)
	t.Cleanup(func() { common.SetCheckpointHook(nil, common.DefaultGapHold) })
	return h
}

// checkReset requires a fresh completed gap and a reset by the next writer.
// Call it only after every other writer and reader has stopped.
func (h *holdTimer) checkReset(t *testing.T, db kvtx.Store, path string, bound int64) {
	// A log already within the bound needs no reset.
	t.Helper()
	info, err := os.Stat(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= bound {
		return
	}

	// Exclude an older gap's delayed completion notice from recovery.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	after := h.passes.Load()
	common.WakeCheckpoint(db)
	for h.gapPass.Load() <= after {
		select {
		case <-h.gaps:
		case <-ctx.Done():
			t.Fatal("the unpinned log did not finish its gap:", ctx.Err())
		}
	}

	// The next writer resets the fully copied log before its own write.
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte("after-gap"), []byte("reset")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(kvtx.WithOrderedCommitContext(ctx)); err != nil {
		t.Fatal(err)
	}

	// Check the configured bound after the commit that consumes the gap.
	info, err = os.Stat(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > bound {
		t.Fatalf("the first commit after the gap left %d WAL bytes, want at most %d", info.Size(), bound)
	}
}

// String reports the holds counted and the longest.
func (h *holdTimer) String() string {
	return strconv.Itoa(int(h.holds.Load())) + " passes held the writer, longest hold " + time.Duration(h.longest.Load()).String()
}

// checkTurn checks, when measuring, that no hold of the writer turn and no
// writer's wait for it passed one gap hold and one slice.
func (h *holdTimer) checkTurn(t *testing.T, turn time.Duration) {
	if !measuring {
		return
	}
	limit := common.FlowWaitMax + common.TurnHold
	if held := time.Duration(h.longest.Load()); held > limit {
		t.Errorf("longest hold of the writer turn was %v, want at most %v", held, limit)
	}
	if turn > limit {
		t.Errorf("longest wait for the writer turn was %v, want at most %v", turn, limit)
	}
}

// walRestartChild writes to the store at path until fillWALGap reaches step,
// prints the commits made, and waits to be killed.
func walRestartChild(t *testing.T, path, step string) {
	// An already-expired deadline interrupts the gap before copying starts.
	hold := common.DefaultGapHold
	if step == "gap false" {
		hold = 0
	}
	var committed atomic.Int64
	common.SetCheckpointHook(func(at string) {
		// Slow each copy for the hold, so the writer outruns the passes.
		if at == "copy" && step == "hold" {
			time.Sleep(100 * time.Millisecond)
		}
		if at == step {
			_, _ = os.Stdout.WriteString("committed " + strconv.FormatInt(committed.Load(), 10) + "\n")
			select {}
		}
	}, hold)
	db := openRestartStore(t, path)
	if err := writeUntilClosed(context.Background(), db, path, &committed); err != nil {
		_, _ = os.Stdout.WriteString("capped: " + err.Error() + "\n")
		os.Exit(1)
	}
}

// TestWALGapEmptyLogRests checks that the checkpointer waits for a commit
// after a pass finds the log empty with its file far past the bound. A
// transaction that spills into a copied log restarts it at once and, rolled
// back, leaves it empty. Only the next commit trims the file. A checkpointer
// that treats the empty pass as a copy to follow up passes again at once,
// thousands of times a second, with no writer.
func TestWALGapEmptyLogRests(t *testing.T) {
	// Count the passes.
	var passes atomic.Int64
	common.SetCheckpointHook(func(step string) {
		if step == "copy" {
			passes.Add(1)
		}
	}, common.DefaultGapHold)
	defer common.SetCheckpointHook(nil, common.DefaultGapHold)

	// Open a store whose cache spills a 4 MiB transaction.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "empty.db")
	db, err := store_kvtx_sqlite.OpenWithPragmas(ctx, path, "kv", store_kvtx_sqlite.Pragmas{WALCheckpointBytes: restartBound, CacheSize: -256})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// write sets rows 4 KiB rows, then commits or discards them.
	write := func(first, rows int, commit bool) {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for j := range rows {
			if err := tx.Set(ctx, restartKey(first+j), restartValue(first+j)); err != nil {
				t.Fatal(err)
			}
		}
		if !commit {
			tx.Discard()
			return
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Commit 8 MiB, 32 times the bound, and let the passes copy it.
	write(0, 2048, true)
	time.Sleep(2 * time.Second)

	// Spill 4 MiB into the copied log, roll it back, and wake the
	// checkpointer once. The checkpointer should then wait.
	write(4096, 1024, false)
	common.WakeCheckpoint(db)
	time.Sleep(200 * time.Millisecond)
	before := passes.Load()
	time.Sleep(time.Second)
	if idle := passes.Load() - before; idle > 5 {
		t.Errorf("checkpointer passed %d times in one second with no writer", idle)
	}
}

// TestWALGapSmallWritersBoundWait checks that a large commit's wait in the
// flow is bounded while small writers commit beside it. Passes held from
// their start let each small writer commit once per walTurnHold, so with
// enough of them every pass finds more than walGapFrames new frames, the gap
// never closes, and the checkpointer never goes idle. The large commit's wait
// then ends once its frames are copied and walFlowWaitMax has passed.
func TestWALGapSmallWritersBoundWait(t *testing.T) {
	ctx := context.Background()
	db := openRestartStore(t, filepath.Join(t.TempDir(), "starve.db"))
	defer db.Close()

	// commit writes rows 4 KiB rows from first in one transaction.
	value := make([]byte, 4096)
	commit := func(ctx context.Context, first, rows int) error {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		for j := range rows {
			if err := tx.Set(ctx, restartKey(first+j), value); err != nil {
				tx.Discard()
				return err
			}
		}
		return tx.Commit(ctx)
	}

	// Beside each count of small writers, time five 8 MiB commits.
	for _, writers := range []int{1, 16, 32} {
		// Start the small writers, 16 rows a commit.
		stop := make(chan struct{})
		done := make(chan struct{})
		for w := range writers {
			go func() {
				defer func() { done <- struct{}{} }()
				for i := 0; ; i++ {
					select {
					case <-stop:
						return
					default:
					}
					_ = commit(ctx, w*512+i*16, 16)
				}
			}()
		}

		// Time the large commits, then stop the small writers. A flow
		// wait ends without failing the commit when its context ends, so
		// a wait the checkpointer never ends shows only as a commit that
		// outlived its deadline. How long the copy takes depends on the
		// disk, so the bound is that the deadline never ends a wait.
		var longest time.Duration
		for i := range 5 {
			c, cancel := context.WithTimeout(ctx, restartTimeCap)
			began := time.Now()
			if err := commit(c, i*2048, 2048); err != nil {
				t.Fatal(err)
			}
			took := time.Since(began)
			longest = max(longest, took)
			if c.Err() != nil {
				t.Errorf("%d small writers: an 8 MiB commit waited %v, until its deadline", writers, took)
			}
			cancel()
		}
		close(stop)
		for range writers {
			<-done
		}
		t.Logf("%d small writers: longest 8 MiB commit %v", writers, longest.Round(time.Millisecond))
	}
}

// openRestartStore opens the store at path with the restart tests' bound.
func openRestartStore(t *testing.T, path string) store_kvtx_sqlite.Store {
	db, err := store_kvtx_sqlite.OpenWithPragmas(context.Background(), path, "kv", store_kvtx_sqlite.Pragmas{WALCheckpointBytes: restartBound})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// writeUntilClosed commits restartKey(i) for each i in turn as an ordered
// commit, which a killed process keeps since SQLite wrote it to the log,
// counting the commits made, until stop ends between two commits or a commit
// fails. It returns an error once it passes a cap: the log at path past
// restartWALCap, restartByteCap written, or restartTimeCap spent.
func writeUntilClosed(stop context.Context, db store_kvtx_sqlite.Store, path string, committed *atomic.Int64) error {
	ctx := context.Background()
	start := time.Now()
	for i := 0; ; i++ {
		// Stop when asked, or at a cap.
		if stop.Err() != nil {
			return nil
		}
		if i*4096 > restartByteCap {
			return errors.New("wrote " + strconv.Itoa(i*4096) + " bytes")
		}
		if time.Since(start) > restartTimeCap {
			return errors.New("wrote for " + restartTimeCap.String())
		}
		if i%64 == 0 {
			if info, err := os.Stat(path + "-wal"); err == nil && info.Size() > restartWALCap {
				return errors.New("log grew to " + strconv.FormatInt(info.Size(), 10) + " bytes")
			}
		}

		// Commit the next key.
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return nil
		}
		if err := tx.Set(ctx, restartKey(i), restartValue(i)); err != nil {
			tx.Discard()
			return nil
		}
		if err := tx.Commit(kvtx.WithOrderedCommitContext(ctx)); err != nil {
			return nil
		}
		committed.Store(int64(i + 1))
	}
}

// checkCommitted reopens the store at path and checks that the last of the
// first n commits to write each key reads back.
func checkCommitted(t *testing.T, path string, n int) {
	ctx := context.Background()
	db := openRestartStore(t, path)
	defer db.Close()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for k := range min(n, restartKeys) {
		// The last commit to write key k.
		i := k + (n-1-k)/restartKeys*restartKeys
		got, found, err := tx.Get(ctx, restartKey(i))
		if err != nil || !found || string(got) != string(restartValue(i)) {
			t.Fatalf("commit %d of %d: found %v, err %v", i, n, found, err)
		}
	}
	t.Logf("%d commits read back", n)
}

// restartKey is the key of commit i.
func restartKey(i int) []byte {
	return []byte("key-" + strconv.Itoa(i%restartKeys))
}

// restartValue is the 4 KiB value of commit i.
func restartValue(i int) []byte {
	return []byte(strings.Repeat(strconv.Itoa(i%10), 4096))
}
