//go:build !tinygo

package block_packsweep_admin

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/writelog"
	"github.com/s4wave/spacewave/net/hash"
)

// mapRoots lists fixed named roots.
type mapRoots map[string]*block.BlockRef

// ListRoots lists every root; the test asks for every prefix.
func (m mapRoots) ListRoots(_ context.Context, _ string, fn func(block.NamedRoot) error) error {
	for name, ref := range m {
		if err := fn(block.NamedRoot{Name: name, Ref: ref}); err != nil {
			return err
		}
	}
	return nil
}

// newRef builds the ref of a block holding data.
func newRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// TestAdminRoots reads the named roots, the pins and the log position, and
// finds a released pin gone and the position empty while the log is off.
func TestAdminRoots(t *testing.T) {
	ctx := t.Context()
	b := newTestBackend(t)
	root := newRef(t, "root")
	pins := NewPins(b.log.Root)
	b.server.opts.Roots = mapRoots{"world/head": root}
	b.server.opts.Pins = pins

	// A pin twice is listed once, beside the root, with the log's position.
	pinned := newRef(t, "snapshot")
	release := pins.Pin(pinned)
	releaseAgain := pins.Pin(pinned)
	refs, position, err := b.admin.WalkRoots(ctx)
	if err != nil || len(refs) != 2 || !refs[0].EqualVT(root) || !refs[1].EqualVT(pinned) {
		t.Fatalf("walk roots = %v, %q, %v", refs, position, err)
	}
	if at, ok := b.log.Position(); !ok || position != at.String() {
		t.Fatalf("position %q, want %v", position, at)
	}

	// Released, the pin is gone.
	release()
	release()
	releaseAgain()
	if refs, _, err = b.admin.WalkRoots(ctx); err != nil || len(refs) != 1 {
		t.Fatalf("walk roots after the release = %v, %v", refs, err)
	}

	// With the log off, no position.
	if err := b.log.Set(nil); err != nil {
		t.Fatal(err)
	}
	if _, position, err = b.admin.WalkRoots(ctx); err != nil || position != "" {
		t.Fatalf("position with the log off = %q, %v", position, err)
	}
}

// TestAdminRootsCoverPins takes pins while Roots runs, and reads the roots
// again just after the log sees each pin: for every answer, each pin taken at
// any point is listed or has a root line after the answer's R.
func TestAdminRootsCoverPins(t *testing.T) {
	ctx := t.Context()
	b := newTestBackend(t)
	b.server.opts.Roots = mapRoots{}

	// Read the roots once the log sees each pin, and record every answer.
	var mtx sync.Mutex
	var answers []*RootsResponse
	readRoots := func() {
		resp, err := b.server.Roots(ctx, &RootsRequest{})
		if err != nil {
			t.Error(err)
			return
		}
		mtx.Lock()
		answers = append(answers, resp)
		mtx.Unlock()
	}
	pins := NewPins(func(h *hash.Hash) {
		b.log.Root(h)
		readRoots()
	})
	b.server.opts.Pins = pins

	// Pin refs in the background while the roots are read.
	var all []*block.BlockRef
	for i := range 400 {
		all = append(all, newRef(t, fmt.Sprintf("pin %d", i)))
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		for _, ref := range all {
			pins.Pin(ref)
		}
	})
	for range 100 {
		readRoots()
	}
	wg.Wait()

	// Check each answer against every pin.
	for i, resp := range answers {
		covered := map[string]bool{}
		for _, ref := range resp.GetPins() {
			covered[ref.GetHash().MarshalString()] = true
		}
		from, err := writelog.ParsePosition(resp.GetPosition())
		if err != nil {
			t.Fatal(err)
		}
		_, err = b.log.ReadLog(ctx, from, 0, func(l writelog.Line) error {
			if l.Kind == writelog.KindRoot {
				covered[l.Hash.MarshalString()] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range all {
			if !covered[ref.GetHash().MarshalString()] {
				t.Fatalf("answer %d at %s neither lists nor logs %s", i, resp.GetPosition(), ref.MarshalString())
			}
		}
	}
}
