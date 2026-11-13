package common_test

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// restartBound is the write-ahead log bound of the restart tests: small, so a
// writer passes it within a checkpoint interval.
const restartBound = 256 << 10

// TestWALRestartStopped stops the store at each step of a restart round,
// killing its process or closing it, reopens it and checks that every commit
// made before the stop reads back.
func TestWALRestartStopped(t *testing.T) {
	// A child process writes until the step it was given, then waits to be
	// killed.
	if env := os.Getenv("SPACEWAVE_WAL_RESTART_CHILD"); env != "" {
		path, step, _ := strings.Cut(env, ",")
		walRestartChild(t, path, step)
		return
	}

	// Kill the process between the passive passes and the restart, and after
	// a restart its deadline interrupted.
	for _, step := range []string{"passed", "interrupted"} {
		t.Run("kill "+step, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kill.db")
			cmd := exec.Command(os.Args[0], "-test.run=^TestWALRestartStopped$", "-test.v")
			cmd.Env = append(os.Environ(), "SPACEWAVE_WAL_RESTART_CHILD="+path+","+step)
			out, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			committed := -1
			for sc := bufio.NewScanner(out); committed < 0 && sc.Scan(); {
				if n, ok := strings.CutPrefix(sc.Text(), "committed "); ok {
					committed, _ = strconv.Atoi(n)
				}
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			if committed < 0 {
				t.Fatal("child exited before the step")
			}
			checkCommitted(t, path, committed)
		})
	}

	// Close the store between the passive passes and the restart.
	t.Run("close passed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "close.db")
		passed := make(chan struct{}, 1)
		common.SetCheckpointHook(func(step string, err error) {
			if step == "passed" {
				select {
				case passed <- struct{}{}:
					time.Sleep(50 * time.Millisecond)
				default:
				}
			}
		}, 25*time.Millisecond)
		defer common.SetCheckpointHook(nil, 25*time.Millisecond)
		db := openRestartStore(t, path)
		var committed atomic.Int64
		writeDone := make(chan struct{})
		go func() {
			defer close(writeDone)
			writeUntilClosed(db, &committed)
		}()
		<-passed
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		<-writeDone
		checkCommitted(t, path, int(committed.Load()))
	})
}

// walRestartChild writes to the store at path until the restart round
// reaches step, prints the commits made, and waits to be killed.
func walRestartChild(t *testing.T, path, step string) {
	// A deadline of 1 µs interrupts every restart.
	hold := 25 * time.Millisecond
	if step == "interrupted" {
		hold = time.Microsecond
	}
	var committed atomic.Int64
	common.SetCheckpointHook(func(at string, err error) {
		if at == step || (step == "interrupted" && at == "restarted" && err != nil) {
			_, _ = os.Stdout.WriteString("committed " + strconv.FormatInt(committed.Load(), 10) + "\n")
			select {}
		}
	}, hold)
	db := openRestartStore(t, path)
	writeUntilClosed(db, &committed)
}

// openRestartStore opens the store at path with the restart tests' bound.
func openRestartStore(t *testing.T, path string) store_kvtx_sqlite.Store {
	db, err := store_kvtx_sqlite.OpenWithPragmas(context.Background(), path, "kv", store_kvtx_sqlite.Pragmas{WALCheckpointBytes: restartBound})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// writeUntilClosed commits restartKey(i) for each i in turn, counting the
// commits made, until a commit fails.
func writeUntilClosed(db store_kvtx_sqlite.Store, committed *atomic.Int64) {
	ctx := context.Background()
	for i := 0; ; i++ {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return
		}
		if err := tx.Set(ctx, restartKey(i), restartValue(i)); err != nil {
			tx.Discard()
			return
		}
		if err := tx.Commit(ctx); err != nil {
			return
		}
		committed.Store(int64(i + 1))
	}
}

// checkCommitted reopens the store at path and checks that the first n
// commits read back.
func checkCommitted(t *testing.T, path string, n int) {
	ctx := context.Background()
	db := openRestartStore(t, path)
	defer db.Close()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for i := range n {
		got, found, err := tx.Get(ctx, restartKey(i))
		if err != nil || !found || string(got) != string(restartValue(i)) {
			t.Fatalf("commit %d of %d: found %v, err %v", i, n, found, err)
		}
	}
	t.Logf("%d commits read back", n)
}

// restartKey is the key of commit i.
func restartKey(i int) []byte {
	return []byte("key-" + strconv.Itoa(i))
}

// restartValue is the 4 KiB value of commit i.
func restartValue(i int) []byte {
	return []byte(strings.Repeat(strconv.Itoa(i%10), 4096))
}
