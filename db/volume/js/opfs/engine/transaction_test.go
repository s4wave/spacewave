//go:build !js

package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
)

// TestTransactionBlindWritesSerialize proves unobserved writes use commit order.
func TestTransactionBlindWritesSerialize(t *testing.T) {
	ctx := t.Context()
	engine, err := Open(ctx, newDiskBackend(t))
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	first, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []struct {
		tx    kvtx.Tx
		key   string
		value string
	}{
		{tx: first, key: "shared", value: "first"},
		{tx: first, key: "first-only", value: "one"},
		{tx: second, key: "shared", value: "second"},
		{tx: second, key: "second-only", value: "two"},
	} {
		if err := mutation.tx.Set(ctx, []byte(mutation.key), []byte(mutation.value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := first.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	read, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	for key, want := range map[string]string{
		"shared":      "second",
		"first-only":  "one",
		"second-only": "two",
	} {
		value, found, err := read.Get(ctx, []byte(key))
		if err != nil || !found || string(value) != want {
			t.Errorf("get %q = %q, %t, %v; want %q", key, value, found, err, want)
		}
	}
}

// TestTransactionReadSetValidation proves a commit conflicts only with later
// writes to the keys and prefixes it read.
func TestTransactionReadSetValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		read     func(ctx context.Context, tx kvtx.Tx) error
		write    string
		conflict bool
	}{{
		name:  "disjoint key",
		read:  getKey("source"),
		write: "other",
	}, {
		name:     "read key",
		read:     getKey("source"),
		write:    "source",
		conflict: true,
	}, {
		name:     "absent key",
		read:     getKey("missing"),
		write:    "missing",
		conflict: true,
	}, {
		name:     "iterated prefix",
		read:     scanPrefix("p/"),
		write:    "p/new",
		conflict: true,
	}, {
		name:  "outside prefix",
		read:  scanPrefix("p/"),
		write: "q",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			engine, err := Open(ctx, newDiskBackend(t))
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			if err := engine.Apply(ctx, []*Record{{Key: []byte("p/old"), Value: []byte("initial")}, {Key: []byte("source"), Value: []byte("initial")}}); err != nil {
				t.Fatal(err)
			}

			dependent, err := engine.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer dependent.Discard()
			if err := tc.read(ctx, dependent); err != nil {
				t.Fatal(err)
			}
			if err := dependent.Set(ctx, []byte("derived"), []byte("result")); err != nil {
				t.Fatal(err)
			}
			if err := engine.Apply(ctx, []*Record{{Key: []byte(tc.write), Value: []byte("competing")}}); err != nil {
				t.Fatal(err)
			}
			err = dependent.Commit(ctx)
			if tc.conflict != errors.Is(err, kvtx.ErrInvalidSnapshot) || (!tc.conflict && err != nil) {
				t.Fatalf("dependent commit error = %v, want conflict %t", err, tc.conflict)
			}

			_, found, _, err := engine.Get(ctx, []byte("derived"))
			if err != nil || found == tc.conflict {
				t.Fatalf("derived write found = %t, error = %v", found, err)
			}
		})
	}
}

// getKey reads one key through a transaction.
func getKey(key string) func(context.Context, kvtx.Tx) error {
	return func(ctx context.Context, tx kvtx.Tx) error {
		_, _, err := tx.Get(ctx, []byte(key))
		return err
	}
}

// scanPrefix reads every key under a prefix through a transaction.
func scanPrefix(prefix string) func(context.Context, kvtx.Tx) error {
	return func(ctx context.Context, tx kvtx.Tx) error {
		return tx.ScanPrefixKeys(ctx, []byte(prefix), func([]byte) error { return nil })
	}
}

// TestTransactionRetainsSnapshot proves reads remain stable and reclamation resumes.
func TestTransactionRetainsSnapshot(t *testing.T) {
	ctx := t.Context()
	disk := newDiskBackend(t)
	queued := make(chan struct{}, 1)
	backend := &queuedReclaimBackend{Backend: disk, queued: queued}
	engine, err := Open(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if err := engine.Apply(ctx, []*Record{{Key: []byte("key"), Value: []byte("before")}}); err != nil {
		t.Fatal(err)
	}

	for _, commit := range []bool{false, true} {
		tx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		value, found, err := tx.Get(ctx, []byte("key"))
		if err != nil || !found {
			t.Fatalf("initial read: %q, %t, %v", value, found, err)
		}
		reads := disk.reads
		for range 100 {
			if _, _, err := tx.Get(ctx, []byte("key")); err != nil {
				t.Fatal(err)
			}
		}
		if disk.reads != reads {
			t.Fatalf("repeated snapshot reads performed %d extra file reads", disk.reads-reads)
		}

		reclaimed := make(chan error, 1)
		go func() {
			_, err := engine.Reclaim(ctx)
			reclaimed <- err
		}()
		select {
		case <-queued:
		case <-time.After(5 * time.Second):
			t.Fatal("reclamation did not queue")
		}
		if err := engine.Apply(ctx, []*Record{{Key: []byte("key"), Value: []byte("after")}}); err != nil {
			t.Fatal(err)
		}
		if got, found, err := tx.Get(ctx, []byte("key")); err != nil || !found || string(got) != string(value) {
			t.Fatalf("snapshot changed during publication: %q, %t, %v", got, found, err)
		}
		select {
		case err := <-reclaimed:
			t.Fatalf("reclamation bypassed live snapshot: %v", err)
		default:
		}
		if commit {
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		} else {
			tx.Discard()
		}
		select {
		case err := <-reclaimed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("reclamation did not resume after snapshot release")
		}
	}
}
