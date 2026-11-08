package common_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/cgo"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// TestReadTurnsLeaveForCallbacks proves a read leaves its turn before a
// scan's callback and between an iterator's steps: with one turn, a scan and
// an iteration whose bodies read in a second transaction both finish.
func TestReadTurnsLeaveForCallbacks(t *testing.T) {
	ctx := context.Background()
	store, err := cgo.Open(ctx, filepath.Join(t.TempDir(), "turns.db"), "kv")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()
	write, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	for i := range 5 {
		if err := write.Set(ctx, []byte("key-"+strconv.Itoa(i)), []byte("value")); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := write.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	common.SetReadTurns(store, 1)

	// Read a key in a transaction of its own.
	readOne := func() error {
		tx, err := store.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		defer tx.Discard()
		_, _, err = tx.Get(ctx, []byte("key-0"))
		return err
	}
	done := make(chan error, 1)
	go func() {
		outer, err := store.NewTransaction(ctx, false)
		if err != nil {
			done <- err
			return
		}
		defer outer.Discard()
		scanned := 0
		err = outer.ScanPrefix(ctx, []byte("key-"), func(key, value []byte) error {
			scanned++
			return readOne()
		})
		if err == nil && scanned != 5 {
			err = os.ErrNotExist
		}
		if err != nil {
			done <- err
			return
		}
		it := outer.Iterate(ctx, []byte("key-"), true, false)
		defer it.Close()
		for it.Next() {
			if err := readOne(); err != nil {
				done <- err
				return
			}
		}
		done <- it.Err()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err.Error())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a read waited for the turn its own scan held")
	}
}

// _ is a type assertion.
var _ kvtx.Store = (*cgo.Store)(nil)
