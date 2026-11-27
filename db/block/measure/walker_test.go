package measure

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// TestMeasureResumeCountsSharedBlockOnce checks a crash between a parent's
// completion and its child's read, an extra named root and a shared hole.
func TestMeasureResumeCountsSharedBlockOnce(t *testing.T) {
	// Make a diamond plus a root whose leaf the other roots cannot reach.
	ctx := t.Context()
	var pack []byte
	type place struct {
		off int64
		n   int
	}
	locations := map[string]place{}
	put := func(data string, kids ...*block.BlockRef) *block.BlockRef {
		// Encode a real BlockObject and remember only the fixture extent.
		ref, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := block.EncodeBlockObject([]byte(data), kids, true)
		if err != nil {
			t.Fatal(err)
		}
		locations[ref.MarshalString()] = place{int64(len(pack)), len(raw)}
		pack = append(pack, raw...)
		return ref
	}
	hole, err := block.BuildBlockRef([]byte("absent"), nil)
	if err != nil {
		t.Fatal(err)
	}
	shared := put("shared", hole)
	left := put("left", shared)
	right := put("right", shared, hole)
	extra := put("extra")
	path := filepath.Join(t.TempDir(), "work.db")
	open := func() *Walker {
		// Reopen the same disk state with a deterministic local range source.
		w, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		w.Locate = func(_ context.Context, ref *block.BlockRef) (uint32, int64, int, bool, error) {
			p, ok := locations[ref.MarshalString()]
			return 1, p.off, p.n, ok, nil
		}
		w.Describe = func(context.Context, *block.BlockRef, []byte, []byte) (string, []Child, error) {
			return "fixture", nil, nil
		}
		w.ReadRange = func(_ context.Context, _ string, off int64, n int) ([]byte, error) {
			return pack[off : off+int64(n)], nil
		}
		if err = w.AddPack(ctx, 1, "one", uint64(len(pack))); err != nil {
			t.Fatal(err)
		}
		return w
	}
	w := open()
	for name, ref := range map[string]*block.BlockRef{"head": left, "retained": right, "checkpoint": extra} {
		if err := w.Seed(ctx, name, ref, nil); err != nil {
			t.Fatal(err)
		}
	}

	// Stop at the second network read, after the first frontier committed.
	read := w.ReadRange
	calls := 0
	w.ReadRange = func(ctx context.Context, id string, off int64, n int) ([]byte, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("stopped")
		}
		return read(ctx, id, off, n)
	}
	if err := w.Run(ctx); err == nil {
		t.Fatal("walk did not stop")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Resume without seeding again; exactly four blocks must remain in the union.
	w = open()
	defer w.Close()
	if err := w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := w.Report(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "total\t4\t20\n") || !strings.Contains(out.String(), "pending\t0\t0\n") || !strings.Contains(out.String(), "holes:index-unresolved\t1\t0\n") || strings.Count(out.String(), "hole:index-unresolved\t"+hole.MarshalString()+"\t") != 2 {
		t.Fatalf("wrong resumed union:\n%s", out.String())
	}
}

// TestSourceHoleReportsParents preserves both root parents when a frozen
// extent's source is unavailable instead of stopping with unexplained pending work.
func TestSourceHoleReportsParents(t *testing.T) {
	// The source failure stub represents a 404 or 416 at an indexed location.
	ctx := t.Context()
	w, err := Open(filepath.Join(t.TempDir(), "work.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ref, err := block.BuildBlockRef([]byte("absent source"), nil)
	if err != nil {
		t.Fatal(err)
	}
	w.Locate = func(context.Context, *block.BlockRef) (uint32, int64, int, bool, error) { return 1, 0, 1, true, nil }
	w.ReadRange = func(context.Context, string, int64, int) ([]byte, error) { return nil, block.ErrNotFound }
	if err = w.AddPack(ctx, 1, "missing", 1); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"head", "checkpoint"} {
		if err = w.Seed(ctx, name, ref, nil); err != nil {
			t.Fatal(err)
		}
	}

	// An unavailable extent is counted once and names all roots that retain it.
	if err = w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = w.Report(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "holes:source-unavailable\t1\t0\n") || strings.Count(out.String(), "hole:source-unavailable\t"+ref.MarshalString()+"\troot:") != 2 {
		t.Fatalf("missing source-hole parents:\n%s", out.String())
	}
}
