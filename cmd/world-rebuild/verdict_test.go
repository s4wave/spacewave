package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
)

// countFixture builds the selftest store, runs count on it with -needed-out,
// and copies it raw, returning the opts, count's result and copy's stats.
func countFixture(t *testing.T) (opts, *countResult, copyStats) {
	t.Helper()
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID, o.chunk, o.readers, o.read, o.txnRows = defaultStoreID, 500, 2, "raw", 20000
	o.walMax = 2560
	if _, err := buildTestStore(o.world, o.packs, 600, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	co := o
	co.neededOut = filepath.Join(dir, "predicted.txt")
	res, err := runCount(co)
	if err != nil {
		t.Fatal("count:", err)
	}
	var st copyStats
	o.copyStatsOut = &st
	if err := runCopy(o); err != nil {
		t.Fatal("copy:", err)
	}
	o.copyStatsOut = nil
	o.neededOut = co.neededOut
	return o, res, st
}

// TestCountNeededOut checks count -needed-out writes the packs the rule's
// dropped blocks need, the list a packcheck of the finished copy finds, and
// that packcheck -needed checks it against a listing.
func TestCountNeededOut(t *testing.T) {
	o, _, _ := countFixture(t)
	predicted, _, err := readNeeded(o.neededOut)
	if err != nil {
		t.Fatal("count -needed-out:", err)
	}
	po := o
	po.list = filepath.Join(filepath.Dir(o.out), "list.txt")
	po.neededOut = filepath.Join(filepath.Dir(o.out), "after.txt")
	writeList(t, po.list, fmt.Sprintf("%d;p/packs/packA", testPackSizes[0]), fmt.Sprintf("%d;p/packs/packB", testPackSizes[1]), "300;p/entries/packA", "300;p/entries/packB")
	if _, err := runPackCheck(po); err != nil {
		t.Fatal("packcheck of the copy:", err)
	}
	after, _, err := readNeeded(po.neededOut)
	if err != nil {
		t.Fatal(err)
	}
	if len(predicted) != 2 || !slices.Equal(predicted, after) {
		t.Fatalf("count predicted %+v, packcheck of the copy found %+v", predicted, after)
	}
	if _, err := runPackCheck(opts{needed: o.neededOut, list: po.list}); err != nil {
		t.Fatal("packcheck -needed of count's list:", err)
	}
}

// TestCopyVerdictsMatchCount checks copy tallies its block keys by verdict
// as count does, so the two can be compared.
func TestCopyVerdictsMatchCount(t *testing.T) {
	_, res, st := countFixture(t)
	if len(st.verdicts) == 0 {
		t.Fatal("copy tallied no verdicts")
	}
	for _, v := range sortedKeys(res.blocks) {
		c, ok := st.verdicts[v]
		if !ok || *c != *res.blocks[v] {
			t.Errorf("%s: copy %+v, count %+v", v, c, res.blocks[v])
		}
	}
	for v := range st.verdicts {
		if _, ok := res.blocks[v]; !ok {
			t.Errorf("%s: copy %+v, count has none", v, st.verdicts[v])
		}
	}
}
