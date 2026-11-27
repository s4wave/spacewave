package packsweep

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/writelog"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// relinkRefs serves the fixture's refs and the refs of blocks put after the
// snapshot, which no catalogued pack holds.
type relinkRefs struct {
	// f is the fixture.
	f *fixture
	// put maps each later block's marshaled ref to its children.
	put map[string][]*block.BlockRef
}

// BlockRefs serves the later blocks, then the fixture's backend blocks.
func (r *relinkRefs) BlockRefs(ctx context.Context, refs []*block.BlockRef) ([]RefsResult, error) {
	out, err := r.f.BlockRefs(ctx, refs)
	if err != nil {
		return nil, err
	}
	for i, ref := range refs {
		if children, ok := r.put[ref.MarshalString()]; ok {
			out[i] = RefsResult{Refs: children}
		}
	}
	return out, nil
}

// newLog turns a write log on in a temporary directory of small segments.
func newLog(t *testing.T) (*writelog.Log, string) {
	t.Helper()
	dir := t.TempDir()
	log := writelog.New(logrus.NewEntry(logrus.New()))
	if err := log.Set(&writelog.Options{Dir: dir, SegmentBytes: 1024}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	return log, dir
}

// position returns the log's position, failing when it is off.
func position(t *testing.T, log *writelog.Log) writelog.Position {
	t.Helper()
	pos, ok := log.Position()
	if !ok {
		t.Fatal("write log is off")
	}
	return pos
}

// countRows counts the keys under prefix.
func countRows(t *testing.T, store kvtx.Store, prefix string) int {
	t.Helper()
	ctx := context.Background()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	var n int
	if err := tx.ScanPrefixKeys(ctx, []byte(prefix), func([]byte) error {
		n++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestClosureKeepsRelinkedSubtree re-links an old dead subtree under a block
// put after the roots were read. The pull stores the put's key, the closure
// finds the whole subtree in W, and the judgment keeps the packs holding it,
// which it judges dead without the log.
func TestClosureKeepsRelinkedSubtree(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 11, fixtureOptions{blocks: 400, roots: 2, packBlocks: 10, dupShare: 0.2})
	before := f.expect(t, 30)

	// Pick an old dead block with children in a dead pack.
	var dead *fixtureBlock
	for id, v := range before.verdicts {
		if v != VerdictDead {
			continue
		}
		for _, b := range f.packs[id] {
			if len(b.refs) != 0 && (dead == nil || b.ref.MarshalString() < dead.ref.MarshalString()) {
				dead = b
			}
		}
	}
	if dead == nil {
		t.Fatal("fixture has no dead block with children")
	}

	// The block put after R links it.
	relinkData := []byte("relink")
	relink, err := block.BuildBlockRef(relinkData, nil)
	if err != nil {
		t.Fatal(err)
	}
	refs := &relinkRefs{f: f, put: map[string][]*block.BlockRef{relink.MarshalString(): {dead.ref}}}

	// Snapshot, read R, and walk. Lines before R are not the cycle's.
	store := store_kvtx_inmem.NewStore()
	later := func() time.Time { return time.Now().Add(48 * time.Hour) }
	if _, err := Snapshot(ctx, &SnapshotOptions{Store: store, Packs: f.store, Reads: 3, Now: later}); err != nil {
		t.Fatal(err)
	}
	log, _ := newLog(t)
	log.Put(dead.ref.GetHash())
	start := position(t, log)
	if _, err := Walk(ctx, &WalkOptions{Store: store, Roots: f.roots, Refs: refs, LogPosition: start.String()}); err != nil {
		t.Fatal(err)
	}

	// After R: the re-linking put, and enough puts and fills of live
	// blocks to span segments.
	log.Put(relink.GetHash())
	lines := uint64(1)
	for _, b := range f.order {
		if before.live[b.ref.MarshalString()] && lines < 120 {
			log.Put(b.ref.GetHash())
			log.Fill(b.ref.GetHash())
			lines += 2
		}
	}

	// The pull stores every line, seeds W with the one key L lacks, and
	// trims the log behind it.
	pullOpts := &PullOptions{Store: store, Source: log, Start: start.String(), Lines: 7}
	pull, err := PullLog(ctx, pullOpts)
	if err != nil {
		t.Fatal(err)
	}
	if pull.Lines != lines || pull.Seeds != 1 || pull.Gap != "" || pull.Position != position(t, log).String() {
		t.Fatalf("pull = %+v, want %d lines and 1 seed to %s", pull, lines, position(t, log))
	}
	res, err := log.ReadLog(ctx, start, 0, func(writelog.Line) error { return nil })
	if err != nil || res.State != writelog.ReadGap {
		t.Fatalf("read from R after the trim = %v, %v, want a gap", res, err)
	}

	// The judgment waits for the closure.
	if _, err := Judge(ctx, &JudgeOptions{Store: store, LowLivePercent: 30}); err == nil {
		t.Fatal("judged with keys waiting in W")
	}

	// The closure finds the relinking block and every block the dead block
	// reaches that L lacks.
	head, err := Closure(ctx, &ClosureOptions{Store: store, Refs: refs, Batch: 5})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{relink.MarshalString(): true}
	var reach func(b *fixtureBlock)
	reach = func(b *fixtureBlock) {
		key := b.ref.MarshalString()
		if want[key] || before.live[key] {
			return
		}
		want[key] = true
		for _, child := range b.refs {
			reach(f.blocks[child.MarshalString()])
		}
	}
	reach(dead)
	if len(want) < 3 {
		t.Fatalf("relinked subtree holds %d keys, want a subtree", len(want))
	}
	if got := countRows(t, store, closureVisitedPrefix); got != len(want) || head.Closure+pull.Seeds != uint64(len(want)) {
		t.Fatalf("W holds %d keys, closure added %d, want %d", got, head.Closure, len(want))
	}
	if n := countRows(t, store, closureFrontierPrefix); n != 0 {
		t.Fatalf("W frontier holds %d keys", n)
	}

	// The judgment matches the roots with the dead block added, and keeps
	// the pack it judged dead without the log.
	if _, err := Judge(ctx, &JudgeOptions{Store: store, LowLivePercent: 30}); err != nil {
		t.Fatal(err)
	}
	f.roots = append(f.roots, dead.ref)
	after := f.expect(t, 30)
	verdicts, holes := readResult(t, store)
	for id, v := range after.verdicts {
		if verdicts[id] != v {
			t.Errorf("pack %s: verdict %v, want %v", id, verdicts[id], v)
		}
	}
	if len(holes) != 0 {
		t.Errorf("holes %v", holes)
	}
	for id, blocks := range f.packs {
		for _, b := range blocks {
			if b == dead && verdicts[id] == VerdictDead {
				t.Errorf("pack %s holding the relinked block judged dead", id)
			}
		}
	}

	// A pull and closure with nothing new change nothing; a repeated key
	// adds a line and no seed.
	again, err := PullLog(ctx, pullOpts)
	if err != nil || *again != *pull {
		t.Fatalf("second pull = %+v, %v, want %+v", again, err, pull)
	}
	log.Put(relink.GetHash())
	if again, err = PullLog(ctx, pullOpts); err != nil || again.Lines != pull.Lines+1 || again.Seeds != pull.Seeds {
		t.Fatalf("pull of a repeated key = %+v, %v", again, err)
	}
	if again, err := Closure(ctx, &ClosureOptions{Store: store, Refs: refs}); err != nil || again.Closure != head.Closure {
		t.Fatalf("second closure = %+v, %v", again, err)
	}
}

// TestPullGaps checks the pull refuses another R, and records a gap when the
// log is turned off, its epoch changes, or the walk's R differs.
func TestPullGaps(t *testing.T) {
	ctx := context.Background()
	h, err := block.BuildBlockRef([]byte("gap"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// The first pull needs R, and a later one refuses another.
	store := store_kvtx_inmem.NewStore()
	log, dir := newLog(t)
	start := position(t, log)
	log.Put(h.GetHash())
	if _, err := PullLog(ctx, &PullOptions{Store: store, Source: log}); err == nil {
		t.Fatal("first pull ran without R")
	}
	if _, err := PullLog(ctx, &PullOptions{Store: store, Source: log, Start: start.String()}); err != nil {
		t.Fatal(err)
	}
	other := writelog.Position{Epoch: start.Epoch, Seq: start.Seq + 1}
	if _, err := PullLog(ctx, &PullOptions{Store: store, Source: log, Start: other.String()}); err == nil {
		t.Fatal("pull ran from another R")
	}

	// Closing the epoch ends its cover, read from the directory too.
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	pull, err := PullLog(ctx, &PullOptions{Store: store, Source: writelog.Dir(dir)})
	if err != nil || !strings.Contains(pull.Gap, "closed") || pull.Lines != 1 {
		t.Fatalf("pull after close = %+v, %v", pull, err)
	}
	if again, err := FollowLog(ctx, &PullOptions{Store: store, Source: writelog.Dir(dir)}, time.Millisecond); err != nil || *again != *pull {
		t.Fatalf("follow after the gap = %+v, %v", again, err)
	}

	// A new epoch loses the old one's lines.
	store = store_kvtx_inmem.NewStore()
	log, _ = newLog(t)
	start = position(t, log)
	if _, err := PullLog(ctx, &PullOptions{Store: store, Source: log, Start: start.String()}); err != nil {
		t.Fatal(err)
	}
	log.Put(h.GetHash())
	if err := log.Set(nil); err != nil {
		t.Fatal(err)
	}
	if err := log.Set(&writelog.Options{Dir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if pull, err = PullLog(ctx, &PullOptions{Store: store, Source: log}); err != nil || pull.Gap == "" || pull.Lines != 0 {
		t.Fatalf("pull after a new epoch = %+v, %v", pull, err)
	}
}

// TestClosureNeedsWalkR checks the closure refuses a pull started at another
// position than the walk's roots.
func TestClosureNeedsWalkR(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 3, fixtureOptions{blocks: 40, roots: 1, packBlocks: 10})
	store := store_kvtx_inmem.NewStore()
	later := func() time.Time { return time.Now().Add(48 * time.Hour) }
	if _, err := Snapshot(ctx, &SnapshotOptions{Store: store, Packs: f.store, Now: later}); err != nil {
		t.Fatal(err)
	}
	log, _ := newLog(t)
	start := position(t, log)
	if _, err := Closure(ctx, &ClosureOptions{Store: store}); err == nil {
		t.Fatal("closure ran before the walk")
	}
	if _, err := Walk(ctx, &WalkOptions{Store: store, Roots: f.roots, LogPosition: start.String()}); err != nil {
		t.Fatal(err)
	}
	log.Put(f.order[0].ref.GetHash())
	other := writelog.Position{Epoch: start.Epoch, Seq: start.Seq + 1}
	if _, err := PullLog(ctx, &PullOptions{Store: store, Source: log, Start: other.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err := Closure(ctx, &ClosureOptions{Store: store}); err == nil {
		t.Fatal("closure ran on a pull from another R")
	}
}

// TestLogBlocking checks a cycle the log does not cover from R blocks removal.
func TestLogBlocking(t *testing.T) {
	walked := &Head{LogPosition: "6f1c2a3b4d5e6f70/42"}
	for _, c := range []struct {
		head *Head
		pull *LogPull
		want string
	}{
		{&Head{}, &LogPull{}, BlockNoLog},
		{walked, nil, BlockLogUnread},
		{walked, &LogPull{Gap: "epoch closed"}, BlockLogGap},
		{walked, &LogPull{Start: walked.LogPosition}, ""},
	} {
		if got := LogBlocking(c.head, c.pull); got != c.want {
			t.Errorf("LogBlocking(%+v, %+v) = %q, want %q", c.head, c.pull, got, c.want)
		}
	}
}
