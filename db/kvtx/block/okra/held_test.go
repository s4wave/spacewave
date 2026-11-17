package kvtx_block_okra

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestDeferCursorWritesMatchImmediate checks that a tree holding its cursor
// writes answers every read as a tree writing them at once does, and reaches
// the same root, through the pattern a World write uses: read an object's
// cursor, replace its block, and set it back.
func TestDeferCursorWritesMatchImmediate(t *testing.T) {
	ctx := t.Context()
	store := newOkraTestStore()
	type tree struct {
		tx   *Tx
		btx  *block.Transaction
		root *block.Cursor
	}
	open := func(deferred bool) *tree {
		btx, cursor := block.NewTransaction(store, nil, nil, nil)
		cursor.SetBlock(&Root{}, true)
		tx, err := NewTx(ctx, cursor, nil, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		if deferred {
			tx.DeferCursorWrites()
		}
		return &tree{tx: tx, btx: btx, root: cursor}
	}
	held, now := open(true), open(false)
	defer held.tx.Discard()
	defer now.tx.Discard()

	// Each value is a block whose Size field carries the value.
	read := func(tr *tree, key string) (uint64, bool) {
		c, err := tr.tx.GetCursorAtKey(ctx, []byte(key))
		if err != nil {
			t.Fatal(err)
		}
		if c == nil {
			return 0, false
		}
		v, err := block.UnmarshalBlock[Root](ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		return v.GetSize(), true
	}
	set := func(tr *tree, key string, v uint64) {
		// Replace the block at the key's own cursor when it exists, as an
		// object's setRoot does, or set a new cursor.
		c, err := tr.tx.GetCursorAtKey(ctx, []byte(key))
		if err != nil {
			t.Fatal(err)
		}
		if c == nil {
			c = tr.root.Detach(false)
			c.ClearAllRefs()
		}
		c.SetBlock(&Root{Size: v}, true)
		if err := tr.tx.SetCursorAtKey(ctx, []byte(key), c, false); err != nil {
			t.Fatal(err)
		}
	}
	check := func(round int, key string, want uint64, wantFound bool) {
		for _, tr := range []*tree{held, now} {
			exists, err := tr.tx.Exists(ctx, []byte(key))
			if err != nil || exists != wantFound {
				t.Fatalf("round %d %s: exists %v, want %v (%v)", round, key, exists, wantFound, err)
			}
			got, found := read(tr, key)
			if found != wantFound || got != want {
				t.Fatalf("round %d %s: read %d %v, want %d %v", round, key, got, found, want, wantFound)
			}
		}
	}

	rng := rand.New(rand.NewPCG(3, 9))
	values := make(map[string]uint64)
	for round := range 24 {
		for i := range 160 {
			key := fmt.Sprintf("object-%04d", rng.IntN(700))
			switch rng.IntN(8) {
			case 0:
				for _, tr := range []*tree{held, now} {
					if err := tr.tx.Delete(ctx, []byte(key)); err != nil {
						t.Fatal(err)
					}
				}
				delete(values, key)
			case 1:
				want, wantFound := values[key]
				for _, tr := range []*tree{held, now} {
					c, err := tr.tx.DeleteCursorAtKey(ctx, []byte(key))
					if err != nil {
						t.Fatal(err)
					}
					if (c != nil) != wantFound {
						t.Fatalf("round %d %s: deleted cursor %v, want %v", round, key, c != nil, wantFound)
					}
					if c != nil {
						v, err := block.UnmarshalBlock[Root](ctx, c)
						if err != nil || v.GetSize() != want {
							t.Fatalf("round %d %s: deleted value %d, want %d (%v)", round, key, v.GetSize(), want, err)
						}
					}
				}
				delete(values, key)
			default:
				v := uint64(round*1000 + i + 1)
				set(held, key, v)
				set(now, key, v)
				values[key] = v
			}
			if i%5 == 0 {
				probe := fmt.Sprintf("object-%04d", rng.IntN(700))
				want, found := values[probe]
				check(round, probe, want, found)
			}
		}

		// An odd round reads through an iterator, which applies the held
		// writes; an even round flushes them.
		if held.tx.Held() == 0 {
			t.Fatalf("round %d: no writes held", round)
		}
		if round%2 == 1 {
			var keys int
			it := held.tx.Iterate(ctx, []byte("object-"), true, false)
			for it.Next() {
				keys++
			}
			if err := it.Err(); err != nil {
				t.Fatal(err)
			}
			it.Close()
			if keys != len(values) {
				t.Fatalf("round %d: iterated %d keys, want %d", round, keys, len(values))
			}
		} else if err := held.tx.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		if held.tx.Held() != 0 {
			t.Fatalf("round %d: %d writes held after the flush", round, held.tx.Held())
		}
		if held.tx.root.GetSize() != now.tx.root.GetSize() || !bytes.Equal(held.tx.root.GetRootHash(), now.tx.root.GetRootHash()) {
			t.Fatalf("round %d: held root differs from the immediate root", round)
		}
	}

	// Writes held at commit land in the written root.
	set(held, "object-last", 1)
	set(now, "object-last", 1)
	for _, tr := range []*tree{held, now} {
		if err := tr.tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	heldRef, _, err := held.btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	nowRef, _, err := now.btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !heldRef.EqualsRef(nowRef) {
		t.Fatal("written roots differ")
	}
}
