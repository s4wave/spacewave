package measure

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

	// Report sorts must spill to disk rather than collecting the graph in RAM.
	var tempStore int
	if err := w.DB.QueryRowContext(ctx, "PRAGMA temp_store").Scan(&tempStore); err != nil || tempStore != 1 {
		t.Fatalf("temp_store=%d err=%v", tempStore, err)
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

// TestMeasureTerminalHoles resumes every terminal hole without downloading its range again.
func TestMeasureTerminalHoles(t *testing.T) {
	for _, kind := range []string{"corrupt", "extent", "refs-unknown", "decode", "repaired"} {
		t.Run(kind, func(t *testing.T) {
			// Build one actual extent, with only the selected boundary damaged.
			ctx := t.Context()
			ref, err := block.BuildBlockRef([]byte("payload"), nil)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := block.EncodeBlockObject([]byte("payload"), nil, kind != "refs-unknown" && kind != "repaired")
			if err != nil {
				t.Fatal(err)
			}
			if kind == "corrupt" {
				raw = []byte("bad")
			}
			path := filepath.Join(t.TempDir(), "work.db")
			reads := 0
			open := func() *Walker {
				w, err := Open(path)
				if err != nil {
					t.Fatal(err)
				}
				w.Locate = func(context.Context, *block.BlockRef) (uint32, int64, int, bool, error) {
					return 1, 0, len(raw), true, nil
				}
				w.ReadRange = func(context.Context, string, int64, int) ([]byte, error) { reads++; return raw, nil }
				w.Describe = func(context.Context, *block.BlockRef, []byte, []byte) (string, []Child, error) {
					if kind == "decode" {
						return "hole:decode", nil, nil
					}
					return "untyped", nil, nil
				}
				w.Children = func(context.Context, *block.BlockRef) ([]*block.BlockRef, bool, error) {
					return nil, kind == "repaired", nil
				}
				size := len(raw)
				if kind == "extent" {
					size--
				}
				if err = w.AddPack(ctx, 1, "pack", uint64(size)); err != nil {
					t.Fatal(err)
				}
				return w
			}
			w := open()
			for _, parent := range []string{"head", "retained"} {
				if err = w.Seed(ctx, parent, ref, nil); err != nil {
					t.Fatal(err)
				}
			}

			// Completing the walk records both parents and a terminal classification.
			if err = w.Run(ctx); err != nil {
				t.Fatal(err)
			}
			var report bytes.Buffer
			if err = w.Report(ctx, &report); err != nil {
				t.Fatal(err)
			}
			want := "hole:" + kind
			if kind == "repaired" {
				want = "total\t1\t7"
			}
			if !strings.Contains(report.String(), want) {
				t.Fatalf("missing %s: %s", want, report.String())
			}
			if kind != "repaired" && strings.Count(report.String(), want+"\t"+ref.MarshalString()+"\troot:") != 2 {
				t.Fatal(report.String())
			}
			prior := reads
			if err = w.Close(); err != nil {
				t.Fatal(err)
			}

			// A later typed sighting must not reset an already judged range.
			w = open()
			defer w.Close()
			if err = w.Seed(ctx, "typed", ref, []byte("descriptor")); err != nil {
				t.Fatal(err)
			}
			if err = w.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if reads != prior {
				t.Fatalf("resume downloaded judged range: %d -> %d", prior, reads)
			}
		})
	}
}

// TestMeasureReachingContexts preserves useful children when another World's constructor fails.
func TestMeasureReachingContexts(t *testing.T) {
	// Serve a shared root and one child locally, without any pack requests.
	ctx := t.Context()
	root, err := block.BuildBlockRef([]byte("root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := block.BuildBlockRef([]byte("child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	w.Local = func(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
		data := []byte("root")
		if ref.MarshalString() == child.MarshalString() {
			data = []byte("child")
		}
		return &block.StoredBlock{Data: data, RefsKnown: true}, nil
	}
	w.Locate = func(context.Context, *block.BlockRef) (uint32, int64, int, bool, error) { return 0, 0, 0, false, nil }
	w.Describe = func(_ context.Context, ref *block.BlockRef, _ []byte, typ []byte) (string, []Child, error) {
		if string(typ) == "bad" {
			return "hole:decode", nil, nil
		}
		if ref.MarshalString() == root.MarshalString() {
			return "fixture", []Child{{Ref: child}}, nil
		}
		return "fixture", nil, nil
	}
	for _, typ := range []string{"bad", "good"} {
		if err = w.Seed(ctx, typ, root, []byte(typ)); err != nil {
			t.Fatal(err)
		}
	}

	// The failed context remains visible while the successful one reaches its child.
	if err = w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = w.Report(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "total\t1\t5") || !strings.Contains(out.String(), "hole:decode\t"+root.MarshalString()) {
		t.Fatal(out.String())
	}
}

// TestMeasureConfigurationStopPersists journals the judged range before refusing every resume.
func TestMeasureConfigurationStopPersists(t *testing.T) {
	// Represent the reader's third 404 without a network service.
	path := filepath.Join(t.TempDir(), "work")
	ref, err := block.BuildBlockRef([]byte("missing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	for attempt := range 2 {
		w, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		w.Locate = func(context.Context, *block.BlockRef) (uint32, int64, int, bool, error) { return 1, 0, 1, true, nil }
		w.ReadRange = func(context.Context, string, int64, int) ([]byte, error) { reads++; return nil, ErrSourceConfiguration }
		if err = w.AddPack(t.Context(), 1, "missing", 1); err != nil {
			t.Fatal(err)
		}
		if err = w.Seed(t.Context(), "head", ref, nil); err != nil {
			t.Fatal(err)
		}

		// Both the original stop and the resumed stop retain one source-hole judgment.
		if err = w.Run(t.Context()); !errors.Is(err, ErrSourceConfiguration) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		var out bytes.Buffer
		if err = w.Report(t.Context(), &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "holes:source-unavailable\t1") {
			t.Fatal(out.String())
		}
		if err = w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if reads != 1 {
		t.Fatalf("configuration resume sent %d requests", reads)
	}
}

// TestMeasureLocalSurvivesInvalidIndex preserves a valid local closure without overstating pack residency.
func TestMeasureLocalSurvivesInvalidIndex(t *testing.T) {
	// A real local block has an impossible extent in the frozen pack catalog.
	w, err := Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ref, err := block.BuildBlockRef([]byte("local"), nil)
	if err != nil {
		t.Fatal(err)
	}
	w.Local = func(context.Context, *block.BlockRef) (*block.StoredBlock, error) {
		return &block.StoredBlock{Data: []byte("local"), RefsKnown: true}, nil
	}
	w.Locate = func(context.Context, *block.BlockRef) (uint32, int64, int, bool, error) { return 1, 99, 5, true, nil }
	w.Describe = func(context.Context, *block.BlockRef, []byte, []byte) (string, []Child, error) {
		return "fixture", nil, nil
	}
	if err = w.AddPack(t.Context(), 1, "pack", 10); err != nil {
		t.Fatal(err)
	}
	if err = w.Seed(t.Context(), "head", ref, nil); err != nil {
		t.Fatal(err)
	}

	// The block is measured, its location is a hole, and no pack live bytes are invented.
	if err = w.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = w.Report(t.Context(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "total\t1\t5") || !strings.Contains(out.String(), "holes:pack-location\t1") || strings.Contains(out.String(), "pack\tpack\t") {
		t.Fatal(out.String())
	}
}

// TestMeasureBoundsWAL walks a long chain, one committed run per block, and
// requires the scratch WAL to stay near its bound instead of keeping every
// frame of the walk.
func TestMeasureBoundsWAL(t *testing.T) {
	// Serve a 1000-block chain locally, so each frontier run holds one block.
	ctx := t.Context()
	prev := walBound
	walBound = 64 << 10
	defer func() { walBound = prev }()
	const n = 1000
	data := func(i int) []byte { return []byte("chain-" + strconv.Itoa(i)) }
	refs := make([]*block.BlockRef, n+1)
	for i := n - 1; i >= 0; i-- {
		ref, err := block.BuildBlockRef(data(i), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
	}
	next := map[string]int{}
	for i := range n {
		next[refs[i].MarshalString()] = i
	}
	path := filepath.Join(t.TempDir(), "work")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	w.Local = func(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
		i, ok := next[ref.MarshalString()]
		if !ok {
			return nil, nil
		}
		var kids []*block.BlockRef
		if refs[i+1] != nil {
			kids = []*block.BlockRef{refs[i+1]}
		}
		return &block.StoredBlock{Data: data(i), Refs: kids, RefsKnown: true}, nil
	}
	w.Locate = func(context.Context, *block.BlockRef) (uint32, int64, int, bool, error) { return 0, 0, 0, false, nil }
	w.Describe = func(context.Context, *block.BlockRef, []byte, []byte) (string, []Child, error) {
		return "fixture", nil, nil
	}
	if err = w.Seed(ctx, "head", refs[0], nil); err != nil {
		t.Fatal(err)
	}
	if err = w.Run(ctx); err != nil {
		t.Fatal(err)
	}

	// Every block counts, and the log holds at most a few runs past its bound.
	var count int
	if err = w.DB.QueryRowContext(ctx, "SELECT count(*) FROM nodes WHERE state=1").Scan(&count); err != nil || count != n {
		t.Fatalf("visited %d of %d: %v", count, n, err)
	}
	info, err := os.Stat(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 4*walBound {
		t.Fatalf("WAL is %d bytes after the walk, bound %d", info.Size(), walBound)
	}
}

// TestMeasureResumeMatchesUninterrupted stops a shared, typed DAG at every
// range read and at every index lookup, which lands both inside a level and
// inside a sighting merge, and requires each resumed walk to report exactly
// what one uninterrupted walk reports.
func TestMeasureResumeMatchesUninterrupted(t *testing.T) {
	// Three levels share children, so a merge files each child from several parents.
	var pack []byte
	type place struct {
		off int64
		n   int
	}
	locations := map[string]place{}
	put := func(data string, kids ...*block.BlockRef) *block.BlockRef {
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
	var leaves, mids, tops []*block.BlockRef
	for i := range 6 {
		leaves = append(leaves, put("leaf-"+strconv.Itoa(i)))
	}
	for i := range 12 {
		mids = append(mids, put("mid-"+strconv.Itoa(i), leaves[i%6], leaves[(i+1)%6], hole))
	}
	for i := range 8 {
		tops = append(tops, put("top-"+strconv.Itoa(i), mids[i%12], mids[(i+3)%12], mids[(i+7)%12]))
	}
	root := put("root", tops...)

	// Small chunks let a stopped lookup land between two merge commits.
	prev := mergeChunk
	mergeChunk = 3
	defer func() { mergeChunk = prev }()

	// walk runs to completion, failing once at the given read or lookup.
	walk := func(failRead, failLocate int) (string, bool) {
		path := filepath.Join(t.TempDir(), "work.db")
		reads, locates := 0, 0
		merging, seeded := false, false
		open := func() *Walker {
			w, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			w.Locate = func(_ context.Context, ref *block.BlockRef) (uint32, int64, int, bool, error) {
				if seeded {
					locates++
				}
				if seeded && locates == failLocate {
					return 0, 0, 0, false, errors.New("stopped lookup")
				}
				p, ok := locations[ref.MarshalString()]
				return 1, p.off, p.n, ok, nil
			}
			w.ReadRange = func(_ context.Context, _ string, off int64, n int) ([]byte, error) {
				reads++
				if reads == failRead {
					return nil, errors.New("stopped read")
				}
				return pack[off : off+int64(n)], nil
			}
			// The root types its tops, so each top also carries a descriptor context.
			w.Describe = func(_ context.Context, ref *block.BlockRef, _ []byte, _ []byte) (string, []Child, error) {
				if ref.MarshalString() != root.MarshalString() {
					return "fixture", nil, nil
				}
				kids := make([]Child, 0, len(tops))
				for _, top := range tops {
					kids = append(kids, Child{Ref: top, Type: []byte("top")})
				}
				return "fixture", kids, nil
			}
			if err = w.AddPack(t.Context(), 1, "one", uint64(len(pack))); err != nil {
				t.Fatal(err)
			}
			return w
		}
		w := open()
		if err := w.Seed(t.Context(), "head", root, nil); err != nil {
			t.Fatal(err)
		}
		seeded = true
		for attempt := 0; ; attempt++ {
			err := w.Run(t.Context())
			if err == nil {
				break
			}
			if attempt != 0 {
				t.Fatalf("second stop: %v", err)
			}
			if err = w.DB.QueryRowContext(t.Context(), "SELECT EXISTS(SELECT 1 FROM merge_cursor WHERE child<>'')").Scan(&merging); err != nil {
				t.Fatal(err)
			}
			if err = w.Close(); err != nil {
				t.Fatal(err)
			}
			w = open()
		}
		defer w.Close()
		var out bytes.Buffer
		if err := w.Report(t.Context(), &out); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		slices.Sort(lines)
		return strings.Join(lines, "\n"), merging
	}

	want, _ := walk(0, 0)
	if !strings.Contains(want, "total\t27\t") || !strings.Contains(want, "pending\t0\t0") || !strings.Contains(want, "pending:sightings\t0\t0") {
		t.Fatalf("wrong uninterrupted walk:\n%s", want)
	}
	midMerge := false
	for i := 1; i <= 40; i++ {
		got, _ := walk(i, 0)
		if got != want {
			t.Fatalf("read %d resumed differently:\n%s\nwant:\n%s", i, got, want)
		}
		got, merging := walk(0, i)
		if got != want {
			t.Fatalf("lookup %d resumed differently:\n%s\nwant:\n%s", i, got, want)
		}
		midMerge = midMerge || merging
	}
	if !midMerge {
		t.Fatal("no stop landed inside a merge")
	}
}
