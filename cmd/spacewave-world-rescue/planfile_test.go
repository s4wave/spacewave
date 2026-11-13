//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// writeIndexID writes to path an --index-id sidecar in sha256sum's form,
// with the index's hash made from db and the -wal's from wal, no -wal line
// when wal is empty, and file names under dir; it returns path.
func writeIndexID(t *testing.T, path, db, wal, dir string) string {
	t.Helper()
	body := fmt.Sprintf("%x  %s/view.db\n", sha256.Sum256([]byte(db)), dir)
	if wal != "" {
		body += fmt.Sprintf("%x  %s/view.db-wal\n", sha256.Sum256([]byte(wal)), dir)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeIndex writes an index file and its -wal in dir, holding "index" and
// "wal", and their --index-id sidecar from their real hashes; it returns the
// index's path and the sidecar's.
func writeIndex(t *testing.T, dir string) (string, string) {
	t.Helper()
	path := filepath.Join(dir, "index.db")
	for name, body := range map[string]string{path: "index", path + "-wal": "wal"} {
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path, writeIndexID(t, path+".id", "index", "wal", dir)
}

// dialTrap is a client every use of which fails the test: a request, a SIZE,
// or a hook installed on it.
type dialTrap struct{ t *testing.T }

func (d dialTrap) GetObject(context.Context, string, string) (io.ReadCloser, error) {
	d.t.Error("GetObject on the trap client")
	return nil, errors.New("trap")
}

func (d dialTrap) GetObjectRange(context.Context, string, string, int64, int) ([]byte, error) {
	d.t.Error("GetObjectRange on the trap client")
	return nil, errors.New("trap")
}

func (d dialTrap) ObjectSize(context.Context, string, string) (int64, error) {
	d.t.Error("ObjectSize on the trap client")
	return 0, errors.New("trap")
}

func (d dialTrap) OnRedial(func(context.Context, error) error) {
	d.t.Error("redial hook installed on the trap client")
}

func (d dialTrap) OnSpareError(func(error)) {
	d.t.Error("spare dial hook installed on the trap client")
}

// groupOf runs group over o as run would, with client, and returns its
// packs, multi and dropped ids.
func groupOf(t *testing.T, o *options, client objectReader) ([]*packWork, map[string]int, []droppedPack) {
	t.Helper()
	l, err := openLedger(o.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	r := &rescuer{
		o: o, le: logrus.NewEntry(logrus.New()), ledger: l,
		multi: make(map[string]int), multiDone: make(map[string]bool), written: make(map[keyTail]struct{}),
		fetch: newFetcher(client, o.Bucket, o.Fetch, 0),
	}
	packs, err := r.group(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return packs, r.multi, r.dropped
}

// samePlan fails unless the packs match in order, size and wants.
func samePlan(t *testing.T, got, want []*packWork) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d packs, want %d", len(got), len(want))
	}
	for i := range got {
		g, w := got[i], want[i]
		if g.id != w.id || g.entry.GetSizeBytes() != w.entry.GetSizeBytes() || len(g.wants) != len(w.wants) {
			t.Fatalf("pack %d: id match %v, size %d want %d, %d wants want %d",
				i, g.id == w.id, g.entry.GetSizeBytes(), w.entry.GetSizeBytes(), len(g.wants), len(w.wants))
		}
		for j := range g.wants {
			gw, ww := g.wants[j], w.wants[j]
			if !bytes.Equal(gw.key, ww.key) || !gw.ref.EqualVT(ww.ref) || gw.off != ww.off || gw.size != ww.size || gw.extent != ww.extent {
				t.Fatalf("pack %d want %d differs", i, j)
			}
		}
	}
}

// planLineOf returns the plan line of a run's log.
func planLineOf(t *testing.T, log string) string {
	t.Helper()
	for line := range strings.Lines(log) {
		if strings.HasPrefix(line, "plan packs ") {
			return line
		}
	}
	t.Fatalf("no plan line in %q", log)
	return ""
}

// outputKeys returns the block keys the output packs hold, sorted.
func outputKeys(t *testing.T, out string) []string {
	t.Helper()
	var keys []string
	for _, ks := range packIndexes(t, out, "packs/") {
		for _, k := range ks {
			keys = append(keys, string(k))
		}
	}
	slices.Sort(keys)
	return keys
}

// cleanSharedNum returns a sharedNumSource with no bloom false positive.
func cleanSharedNum(t *testing.T) (src *source, a, b, c []*block.BlockRef) {
	t.Helper()
	for try := 0; ; try++ {
		var clean bool
		src, a, b, c, clean = sharedNumSource(context.Background(), t)
		if clean {
			return src, a, b, c
		}
		if try == 20 {
			t.Fatal("every source had a bloom false positive")
		}
	}
}

// TestRescuePlanRoundTrip writes a plan with --plan-out and runs it with
// --plan-in: the plan holds the packs, order, wants, multi and dropped ids
// group gives in process, and the run with it stores the same blocks with
// the same counts and plan line as an in-process run. The fixture has a
// shared number whose second pack has no e row, so a's refs go to both
// candidates, and --packs names a pack no ref places and an unknown id.
func TestRescuePlanRoundTrip(t *testing.T) {
	// Build the source and the options of each run.
	ctx := context.Background()
	src, a, b, c := cleanSharedNum(t)
	idA, idB, idC := packOf(t, src, a[0]), packOf(t, src, b[0]), packOf(t, src, c[0])
	src.dropEntry(idB)
	dir := t.TempDir()
	list := writeList(t, dir, append(append(append([]*block.BlockRef{}, a...), b...), c...))
	indexPath, indexID := writeIndex(t, dir)
	opts := func(out string, log io.Writer) *options {
		o := testOptions(src, list, out)
		o.Packs = map[string]bool{idA: true, idB: true, idC: true, "pfv1_notapack": true}
		o.PlanSize, o.IndexPath, o.IndexID = true, indexPath, indexID
		o.Log = log
		return o
	}

	// Group in process.
	packs0, multi0, dropped0 := groupOf(t, opts(t.TempDir(), io.Discard), &fakeServer{dir: src.dir})
	if len(multi0) == 0 || len(dropped0) != 2 {
		t.Fatalf("fixture: %d shared keys, %d dropped; want some and 2", len(multi0), len(dropped0))
	}

	// Plan out with a client that fails the test on any use: the plan
	// matches group.
	outP := t.TempDir()
	var logP bytes.Buffer
	oP := opts(outP, &logP)
	oP.PlanOut = filepath.Join(dir, "rescue.plan")
	resP, err := run(ctx, oP, dialTrap{t})
	if err != nil || resP.Requests != 0 || resP.PacksTotal != 0 {
		t.Fatalf("plan out: %v, %+v", err, resP)
	}
	pf, err := readPlanFile(oP.PlanOut)
	if err != nil {
		t.Fatal(err)
	}
	samePlan(t, pf.Packs, packs0)
	if !maps.Equal(pf.Multi, multi0) || !slices.Equal(pf.Dropped, dropped0) {
		t.Fatalf("plan multi %v dropped %v, group %v %v", pf.Multi, pf.Dropped, multi0, dropped0)
	}

	// Run the plan on the plan's ledger with no index, and in process on a
	// ledger of its own.
	var logI, logR bytes.Buffer
	oI := opts(outP, &logI)
	oI.PlanIn, oI.Index = oP.PlanOut, nil
	resI, err := run(ctx, oI, &fakeServer{dir: src.dir})
	if err != nil {
		t.Fatal(err)
	}
	outR := t.TempDir()
	resR, err := run(ctx, opts(outR, &logR), &fakeServer{dir: src.dir})
	if err != nil {
		t.Fatal(err)
	}

	// Both store the same blocks with the same counts and plan line.
	type counts struct {
		packs, done, absent, notInPack, ambiguous, shared, deferred, noExt, noExtDone int
		bytes                                                                         uint64
	}
	of := func(r *result) counts {
		return counts{r.PacksTotal, r.Done, r.Absent, r.NotInPack, r.Ambiguous, r.SharedNum, r.Deferred, r.NoExtent, r.NoExtentDone, r.Bytes}
	}
	if of(resI) != of(resR) {
		t.Fatalf("plan in %+v, in process %+v", of(resI), of(resR))
	}
	if lp, li, lr := planLineOf(t, logP.String()), planLineOf(t, logI.String()), planLineOf(t, logR.String()); lp != lr || li != lr {
		t.Fatalf("plan lines differ:\n%s%s%s", lp, li, lr)
	}
	if ki, kr := outputKeys(t, outP), outputKeys(t, outR); len(kr) == 0 || !slices.Equal(ki, kr) {
		t.Fatalf("plan in stored %d keys, in process %d, or other keys", len(ki), len(kr))
	}

	// The in-process run lists its dropped ids beside the output.
	raw, err := os.ReadFile(filepath.Join(outR, droppedListName))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), "\n"); n != 2 {
		t.Fatalf("%d dropped lines, want 2", n)
	}
}

// TestRescuePlanOutNoDial plans with a client every use of which fails the
// test: no request, SIZE or hook. The plan holds the extents group gives,
// and the ledger gets the absent line of a ref the index lacks.
func TestRescuePlanOutNoDial(t *testing.T) {
	// Write two packs and a ref no pack holds.
	src := newSource(t)
	refs := append(src.pack(4), src.pack(3)...)
	data := make([]byte, 300)
	_, _ = rand.Read(data)
	missing, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	src.close()
	dir := t.TempDir()
	list := writeList(t, dir, append(append([]*block.BlockRef{}, refs...), missing))

	// Plan with the trap client.
	out := t.TempDir()
	var log bytes.Buffer
	o := testOptions(src, list, out)
	o.PlanOut, o.PlanSize, o.Log = filepath.Join(dir, "rescue.plan"), true, &log
	o.IndexPath, o.IndexID = writeIndex(t, dir)
	res, err := run(context.Background(), o, dialTrap{t})
	if err != nil || res.Requests != 0 || res.Absent != 1 {
		t.Fatalf("plan out: %v, %+v", err, res)
	}
	if !strings.HasPrefix(planLineOf(t, log.String()), "plan packs 2 wanted 7 ") {
		t.Fatalf("plan line %q", planLineOf(t, log.String()))
	}

	// The plan matches group, extents included.
	pf, err := readPlanFile(o.PlanOut)
	if err != nil {
		t.Fatal(err)
	}
	packs, _, _ := groupOf(t, testOptions(src, list, t.TempDir()), &fakeServer{dir: src.dir})
	samePlan(t, pf.Packs, packs)
	if !pf.Packs[0].wants[0].extent {
		t.Fatal("plan want has no extent")
	}

	// The ledger holds the absent ref.
	raw, err := os.ReadFile(o.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), " absent no-index-row\n") {
		t.Fatalf("ledger %q lacks the absent ref", raw)
	}
}

// TestRescuePlanInResume runs a plan until a drop stops it after the first
// pack, then runs the same plan again: it fetches the other two packs only,
// skipping exactly the packs the ledger finished.
func TestRescuePlanInResume(t *testing.T) {
	// Plan three packs.
	ctx := context.Background()
	src := newSource(t)
	var refs []*block.BlockRef
	for range 3 {
		refs = append(refs, src.pack(4)...)
	}
	src.close()
	out := t.TempDir()
	list := writeList(t, out, refs)
	plan := filepath.Join(out, "rescue.plan")
	indexPath, indexID := writeIndex(t, out)
	oP := dropOptions(src, list, out, io.Discard)
	oP.PlanOut, oP.PlanSize, oP.IndexPath, oP.IndexID = plan, true, indexPath, indexID
	if _, err := run(ctx, oP, dialTrap{t}); err != nil {
		t.Fatal(err)
	}
	withPlan := func() *options {
		o := dropOptions(src, list, out, io.Discard)
		o.PlanIn, o.PlanSize, o.IndexID, o.Index = plan, true, indexID, nil
		return o
	}

	// Drop the connection on the second pack.
	first := &fakeServer{dir: src.dir, hook: func(n int) (time.Duration, error) {
		if n == 2 {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, nil
	}}
	res1, err := run(ctx, withPlan(), first)
	var hard *hardStopError
	if !errors.As(err, &hard) || res1.PacksDone != 1 || res1.Done != 4 {
		t.Fatalf("first run: %v, %+v", err, res1)
	}
	done1 := packLines(t, out)
	if len(done1) != 1 {
		t.Fatalf("pack lines %v, want 1", done1)
	}

	// The second run fetches the two packs the first did not finish.
	var log bytes.Buffer
	o2 := withPlan()
	o2.Log = &log
	second := &fakeServer{dir: src.dir}
	res2, err := run(ctx, o2, second)
	if calls, _ := second.count(); err != nil || calls != 2 || res2.PacksTotal != 2 || res2.Done != 12 {
		t.Fatalf("second run: %v, calls %d, %+v", err, calls, res2)
	}
	if !strings.HasPrefix(planLineOf(t, log.String()), "plan packs 2 wanted 8 ") {
		t.Fatalf("plan line %q", planLineOf(t, log.String()))
	}
	if want := "plan ledger " + o2.Ledger + " p 1 r "; !strings.Contains(log.String(), want) {
		t.Fatalf("log %q; want a line %q", log.String(), want)
	}
	lines := packLines(t, out)
	if len(lines) != 3 {
		t.Fatalf("pack lines %v, want 3", lines)
	}
	for _, f := range lines[1:] {
		if f[1] == done1[0][1] {
			t.Fatal("the finished pack was fetched again")
		}
	}
	if n := checkOutput(t, out, refs); n != 12 {
		t.Fatalf("output holds %d of 12", n)
	}
}

// TestRescuePlanInRefuses refuses, before any request and in main's
// preflight too, a plan made from other refs, --packs, --whole-packs, flags,
// table, store id or index, a run with no --index-id, a torn plan and one of
// another version; refuses a plan out whose sidecar is not the index file
// and -wal; accepts a renamed index with the same sidecar hashes; checks
// checkPacks; and finds no digit in any refusal but its paths.
func TestRescuePlanInRefuses(t *testing.T) {
	// Plan two packs over an index file whose sidecar identity the plan
	// holds.
	ctx := context.Background()
	src := newSource(t)
	a, b := src.pack(4), src.pack(4)
	refs := append(append([]*block.BlockRef{}, a...), b...)
	src.close()
	idA, idB := packOf(t, src, a[0]), packOf(t, src, b[0])
	out := t.TempDir()
	list := writeList(t, out, refs)
	indexPath, indexID := writeIndex(t, out)
	plan := filepath.Join(out, "rescue.plan")
	named := func(o *options) *options {
		o.Packs = map[string]bool{idA: true, idB: true}
		o.PlanSize, o.IndexPath, o.IndexID = true, indexPath, indexID
		o.IndexTable, o.StoreID = defaultIndexTable, "store-a"
		return o
	}
	oP := named(testOptions(src, list, out))
	oP.PlanOut = plan
	if _, err := run(ctx, oP, dialTrap{t}); err != nil {
		t.Fatal(err)
	}
	withPlan := func() *options {
		o := named(testOptions(src, list, out))
		o.PlanIn, o.Index = plan, nil
		return o
	}
	var refusals []string
	refuse := func(name string, o *options, want string) {
		t.Helper()
		srv := &fakeServer{dir: src.dir}
		_, err := run(ctx, o, srv)
		if calls, _ := srv.count(); err == nil || !strings.Contains(err.Error(), want) || calls != 0 {
			t.Fatalf("%s: %v, calls %d; want %q and none", name, err, calls, want)
		}
		refusals = append(refusals, err.Error())
		if o.PlanIn == "" {
			return
		}

		// Main's preflight, before the netrc and the client, refuses the
		// same way, unless the run refused before reading the plan.
		if o.IndexID == "" {
			return
		}
		index, err := readIndexID(o.IndexID)
		if err == nil {
			_, err = checkPlan(o, o.PlanSize, index)
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s preflight: %v; want %q", name, err, want)
		}
	}

	// Other refs, --packs, --whole-packs, flags, table or store id.
	shorter := filepath.Join(t.TempDir(), "refs.txt")
	_ = writeList(t, filepath.Dir(shorter), refs[1:])
	o := withPlan()
	o.RefList = shorter
	refuse("refs", o, "other inputs: refs differ")
	o = withPlan()
	o.Packs = map[string]bool{idA: true}
	refuse("packs", o, "other inputs: packs differ")
	o = withPlan()
	var kept, dropped bytes.Buffer
	o.Keep, o.WholePacks = keepOf(t, b[:1], &kept, &dropped), []string{idB}
	refuse("whole packs", o, "other inputs: whole-packs differ")
	o = withPlan()
	o.MinWanted = 1
	refuse("flags", o, "other inputs: flags differ")
	o = withPlan()
	o.IndexTable = "pack_index_old"
	refuse("index table", o, "other inputs: index-table differ")
	o = withPlan()
	o.StoreID = "store-b"
	refuse("store id", o, "other inputs: store-id differ")

	// A sidecar for another index: one changed in place at the same size,
	// which only its mtime showed before, a changed -wal, and a -wal gone.
	// Then no sidecar, for plan in and plan out.
	for _, c := range []struct{ name, db, wal string }{
		{"index content", "indey", "wal"},
		{"wal content", "index", "wam"},
		{"wal gone", "index", ""},
	} {
		o = withPlan()
		o.IndexID = writeIndexID(t, filepath.Join(t.TempDir(), "other.db.id"), c.db, c.wal, out)
		refuse(c.name, o, "--index-id names another index")
	}
	o = withPlan()
	o.IndexID = ""
	refuse("no index id", o, "need --index-id")
	o = testOptions(src, list, t.TempDir())
	o.PlanOut, o.PlanSize = filepath.Join(t.TempDir(), "rescue.plan"), true
	refuse("plan out with no index id", o, "need --index-id")

	// A plan out whose sidecar is not the index it opened: another index,
	// another -wal, a -wal the sidecar lacks or names but is not there, and
	// no index file.
	planOut := func(db, wal, dir string) *options {
		o := named(testOptions(src, list, t.TempDir()))
		o.PlanOut = filepath.Join(t.TempDir(), "rescue.plan")
		o.IndexPath = filepath.Join(dir, "index.db")
		o.IndexID = writeIndexID(t, filepath.Join(t.TempDir(), "out.id"), db, wal, dir)
		return o
	}
	noWal := t.TempDir()
	if err := os.WriteFile(filepath.Join(noWal, "index.db"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	refuse("plan out index", planOut("indey", "wal", out), "does not match the index file")
	refuse("plan out wal", planOut("index", "wam", out), "does not match the index -wal")
	refuse("plan out wal unnamed", planOut("index", "", out), "does not match the index -wal")
	refuse("plan out wal absent", planOut("index", "wal", noWal), "does not match the index -wal")
	o = planOut("index", "wal", out)
	o.IndexPath = ""
	refuse("plan out no index file", o, "needs the index file")
	if _, err := run(ctx, planOut("index", "", noWal), dialTrap{t}); err != nil {
		t.Fatalf("plan out with no -wal: %v", err)
	}

	// A renamed copy of the index, its sidecar made elsewhere with the same
	// hashes, is accepted and fetched.
	renamed := filepath.Join(t.TempDir(), "copy-of-index.db")
	if err := os.Rename(indexPath, renamed); err != nil {
		t.Fatal(err)
	}
	o = withPlan()
	o.IndexPath = renamed
	o.IndexID = writeIndexID(t, filepath.Join(t.TempDir(), "copy.id"), "index", "wal", "/mnt/elsewhere")
	srv := &fakeServer{dir: src.dir}
	if res, err := run(ctx, o, srv); err != nil || res.Requests == 0 {
		t.Fatalf("renamed index: %v, %+v; want a fetch", err, res)
	}

	// Without a sidecar the identity is the stat one, which a same-size
	// rewrite moves through its mtime and a new -wal moves too.
	stat := func() string {
		t.Helper()
		id, err := indexIdentity(renamed)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	before := stat()
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(renamed, later, later); err != nil {
		t.Fatal(err)
	}
	moved := stat()
	if err := os.WriteFile(renamed+"-wal", []byte("wal"), 0o644); err != nil {
		t.Fatal(err)
	}
	if walled := stat(); before == moved || moved == walled {
		t.Fatalf("stat identity: %q, after mtime %q, after -wal %q", before, moved, walled)
	}

	// A torn plan.
	raw, err := os.ReadFile(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan, raw[:len(raw)-8], 0o644); err != nil {
		t.Fatal(err)
	}
	refuse("torn", withPlan(), "read plan")

	// A plan of another format version.
	var other bytes.Buffer
	other.WriteString(planMagic)
	zw := gzip.NewWriter(&other)
	_, _ = zw.Write([]byte{planVersion + 1})
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan, other.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	refuse("version", withPlan(), "plan format version")

	// checkPacks takes the plan's packs and dropped ids, no more or fewer.
	pf := &planFile{Packs: []*packWork{{id: "x"}}, Dropped: []droppedPack{{id: "y", reason: dropNoWants}}}
	if err := pf.checkPacks(nil); err != nil {
		t.Fatal(err)
	}
	if err := pf.checkPacks(map[string]bool{"x": true, "y": true}); err != nil {
		t.Fatal(err)
	}
	for _, packs := range []map[string]bool{{"x": true}, {"x": true, "y": true, "z": true}} {
		err := pf.checkPacks(packs)
		if err == nil || !strings.Contains(err.Error(), "--packs differs") {
			t.Fatalf("checkPacks %v: %v", packs, err)
		}
		refusals = append(refusals, err.Error())
	}

	// No refusal holds a digit once its paths are taken out, so none can
	// read as a reply code.
	path := regexp.MustCompile(`/[^\s:"]*`)
	for _, msg := range refusals {
		if bare := path.ReplaceAllString(msg, "PATH"); strings.ContainsAny(bare, "0123456789") {
			t.Errorf("refusal %q holds a digit", msg)
		}
	}
}
