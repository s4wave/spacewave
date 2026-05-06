package pagestore

import (
	"bytes"
	"strconv"
	"testing"
)

func TestTreeSingleLeaf(t *testing.T) {
	pager := NewMemPager(DefaultPageSize)
	tree := NewTree(pager)

	if err := tree.Put([]byte("hello"), []byte("world")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, found, err := tree.Get([]byte("hello"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("Get: not found")
	}
	if string(val) != "world" {
		t.Fatalf("Get: got %q", val)
	}

	_, found, err = tree.Get([]byte("missing"))
	if err != nil {
		t.Fatalf("Get(missing): %v", err)
	}
	if found {
		t.Fatal("Get(missing): should not be found")
	}
}

func TestTreeUpdate(t *testing.T) {
	pager := NewMemPager(DefaultPageSize)
	tree := NewTree(pager)

	if err := tree.Put([]byte("key"), []byte("v1")); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := tree.Put([]byte("key"), []byte("v2")); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	val, found, err := tree.Get([]byte("key"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || string(val) != "v2" {
		t.Fatalf("Get: found=%v val=%q", found, val)
	}
}

func TestTreeOverflowValueRoundTrip(t *testing.T) {
	pager := NewMemPager(DefaultPageSize)
	tree := NewTree(pager)
	large := bytes.Repeat([]byte("x"), DefaultPageSize+1024)

	if err := tree.Put([]byte("large"), large); err != nil {
		t.Fatalf("Put large: %v", err)
	}

	val, found, err := tree.Get([]byte("large"))
	if err != nil {
		t.Fatalf("Get large: %v", err)
	}
	if !found {
		t.Fatal("Get large: not found")
	}
	if !bytes.Equal(val, large) {
		t.Fatalf("Get large mismatch: got %d bytes want %d", len(val), len(large))
	}

	var scanned []byte
	if err := tree.ScanPrefix([]byte("lar"), func(key, value []byte) bool {
		scanned = bytes.Clone(value)
		return true
	}); err != nil {
		t.Fatalf("ScanPrefix large: %v", err)
	}
	if !bytes.Equal(scanned, large) {
		t.Fatalf("ScanPrefix large mismatch: got %d bytes want %d", len(scanned), len(large))
	}
}

func TestTreeOverflowManifestBloomAfterLeafSplit(t *testing.T) {
	pager := NewMemPager(DefaultPageSize)
	tree := NewTree(pager)
	key := []byte("pack_bloom/aa/test-pack")
	large := bytes.Repeat([]byte("b"), 5152)

	for i := range 256 {
		k := []byte("packs/aa/test-pack-" + strconv.Itoa(i))
		if err := tree.Put(k, []byte("entry")); err != nil {
			t.Fatalf("Put small %d: %v", i, err)
		}
	}
	if err := tree.Put(key, large); err != nil {
		t.Fatalf("Put manifest bloom: %v", err)
	}

	got, found, err := tree.Get(key)
	if err != nil {
		t.Fatalf("Get manifest bloom: %v", err)
	}
	if !found || !bytes.Equal(got, large) {
		t.Fatalf("Get manifest bloom: found=%v got %d bytes want %d", found, len(got), len(large))
	}
}

func TestTreeLargeMiddleValueUsesOverflowForSplitSafety(t *testing.T) {
	pager := NewMemPager(DefaultPageSize)
	tree := NewTree(pager)

	left := bytes.Repeat([]byte("l"), 1700)
	right := bytes.Repeat([]byte("r"), 1700)
	middle := bytes.Repeat([]byte("m"), 2500)

	if err := tree.Put([]byte("pack_bloom/aa/left"), left); err != nil {
		t.Fatalf("Put left: %v", err)
	}
	if err := tree.Put([]byte("pack_bloom/zz/right"), right); err != nil {
		t.Fatalf("Put right: %v", err)
	}
	if err := tree.Put([]byte("pack_bloom/mm/middle"), middle); err != nil {
		t.Fatalf("Put middle: %v", err)
	}

	got, found, err := tree.Get([]byte("pack_bloom/mm/middle"))
	if err != nil {
		t.Fatalf("Get middle: %v", err)
	}
	if !found || !bytes.Equal(got, middle) {
		t.Fatalf("Get middle: found=%v got %d want %d", found, len(got), len(middle))
	}
}

func TestTreeLeafSplitBalancesEncodedSize(t *testing.T) {
	pager := NewMemPager(1024)
	tree := NewTree(pager)
	for i := range 40 {
		key := []byte("a" + strconv.Itoa(i))
		value := bytes.Repeat([]byte("s"), 16)
		if err := tree.Put(key, value); err != nil {
			t.Fatalf("Put small %d: %v", i, err)
		}
	}
	large := bytes.Repeat([]byte("l"), 650)
	if err := tree.Put([]byte("z"), large); err != nil {
		t.Fatalf("Put large value: %v", err)
	}

	got, found, err := tree.Get([]byte("z"))
	if err != nil {
		t.Fatalf("Get large value: %v", err)
	}
	if !found || !bytes.Equal(got, large) {
		t.Fatalf("Get large value: found=%v got %d want %d", found, len(got), len(large))
	}
}

func TestTreeOverflowValueUpdateDeleteAndSnapshot(t *testing.T) {
	pager := NewMemPager(DefaultPageSize)
	tree := NewTree(pager)
	large := bytes.Repeat([]byte("a"), DefaultPageSize+2048)

	if err := tree.Put([]byte("key"), large); err != nil {
		t.Fatalf("Put large: %v", err)
	}
	rootLarge := tree.RootID()

	small := []byte("small")
	if err := tree.Put([]byte("key"), small); err != nil {
		t.Fatalf("Put small: %v", err)
	}

	val, found, err := tree.Get([]byte("key"))
	if err != nil {
		t.Fatalf("Get small: %v", err)
	}
	if !found || string(val) != string(small) {
		t.Fatalf("Get small: found=%v val=%q", found, val)
	}

	snapLarge := OpenTree(pager, rootLarge)
	val, found, err = snapLarge.Get([]byte("key"))
	if err != nil {
		t.Fatalf("snap Get large: %v", err)
	}
	if !found || !bytes.Equal(val, large) {
		t.Fatalf("snap Get large: found=%v len=%d want %d", found, len(val), len(large))
	}

	found, err = tree.Delete([]byte("key"))
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !found {
		t.Fatal("Delete: not found")
	}
	_, found, err = tree.Get([]byte("key"))
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if found {
		t.Fatal("Get after delete: found")
	}
}

func TestTreeSplit(t *testing.T) {
	// Use a small page size to force splits quickly.
	pager := NewMemPager(256)
	tree := NewTree(pager)

	// Insert enough entries to force multiple splits.
	n := 100
	for i := range n {
		key := "key-" + zeroPad(i, 4)
		val := "val-" + strconv.Itoa(i)
		if err := tree.Put([]byte(key), []byte(val)); err != nil {
			t.Fatalf("Put(%s): %v", key, err)
		}
	}

	// Verify all entries retrievable.
	for i := range n {
		key := "key-" + zeroPad(i, 4)
		val, found, err := tree.Get([]byte(key))
		if err != nil {
			t.Fatalf("Get(%s): %v", key, err)
		}
		if !found {
			t.Fatalf("Get(%s): not found", key)
		}
		expected := "val-" + strconv.Itoa(i)
		if string(val) != expected {
			t.Fatalf("Get(%s): got %q, want %q", key, val, expected)
		}
	}

	// Verify tree has branch pages (rootID should be a branch now).
	if tree.RootID() == InvalidPage {
		t.Fatal("root is invalid after inserts")
	}
}

func TestTreeDelete(t *testing.T) {
	pager := NewMemPager(DefaultPageSize)
	tree := NewTree(pager)

	tree.Put([]byte("a"), []byte("1"))
	tree.Put([]byte("b"), []byte("2"))
	tree.Put([]byte("c"), []byte("3"))

	found, err := tree.Delete([]byte("b"))
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !found {
		t.Fatal("Delete: not found")
	}

	_, found, _ = tree.Get([]byte("b"))
	if found {
		t.Fatal("b should be deleted")
	}

	// a and c should still be there.
	val, found, _ := tree.Get([]byte("a"))
	if !found || string(val) != "1" {
		t.Fatalf("a: found=%v val=%q", found, val)
	}
	val, found, _ = tree.Get([]byte("c"))
	if !found || string(val) != "3" {
		t.Fatalf("c: found=%v val=%q", found, val)
	}
}

func TestTreeScanPrefix(t *testing.T) {
	pager := NewMemPager(DefaultPageSize)
	tree := NewTree(pager)

	tree.Put([]byte("user/alice"), []byte("1"))
	tree.Put([]byte("user/bob"), []byte("2"))
	tree.Put([]byte("user/charlie"), []byte("3"))
	tree.Put([]byte("config/x"), []byte("4"))

	var found []string
	tree.ScanPrefix([]byte("user/"), func(key, value []byte) bool {
		found = append(found, string(key))
		return true
	})

	if len(found) != 3 {
		t.Fatalf("ScanPrefix: got %d, want 3: %v", len(found), found)
	}
}

func TestTreeScanPrefixIncludesBinarySuffixBytes(t *testing.T) {
	pager := NewMemPager(256)
	tree := NewTree(pager)
	keys := [][]byte{
		[]byte("bin/"),
		[]byte("bin/a"),
		{'b', 'i', 'n', '/', 0x7e},
		{'b', 'i', 'n', '/', 0x7f},
		{'b', 'i', 'n', '/', 0xff},
		[]byte("bio/a"),
		[]byte("other"),
	}
	for i := range keys {
		if err := tree.Put(keys[i], []byte{byte(i)}); err != nil {
			t.Fatalf("Put %x: %v", keys[i], err)
		}
	}

	var found [][]byte
	if err := tree.ScanPrefix([]byte("bin/"), func(key, value []byte) bool {
		found = append(found, bytes.Clone(key))
		return true
	}); err != nil {
		t.Fatalf("ScanPrefix: %v", err)
	}

	want := [][]byte{
		[]byte("bin/"),
		[]byte("bin/a"),
		{'b', 'i', 'n', '/', 0x7e},
		{'b', 'i', 'n', '/', 0x7f},
		{'b', 'i', 'n', '/', 0xff},
	}
	if len(found) != len(want) {
		t.Fatalf("ScanPrefix: got %d keys want %d: %x", len(found), len(want), found)
	}
	for i := range want {
		if !bytes.Equal(found[i], want[i]) {
			t.Fatalf("ScanPrefix[%d]: got %x want %x", i, found[i], want[i])
		}
	}
}

func TestTreeScanPrefixStopsAcrossBranchPages(t *testing.T) {
	pager := NewMemPager(256)
	tree := NewTree(pager)
	for i := range 100 {
		key := []byte("key-" + zeroPad(i, 4))
		if err := tree.Put(key, []byte("value")); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
	}

	count := 0
	if err := tree.ScanPrefix([]byte("key-"), func(key, value []byte) bool {
		count++
		return false
	}); err != nil {
		t.Fatalf("ScanPrefix: %v", err)
	}
	if count != 1 {
		t.Fatalf("ScanPrefix callbacks: got %d want 1", count)
	}
}

func TestTreeScanPrefixPrunesBranchPages(t *testing.T) {
	pager := &countingPager{Pager: NewMemPager(256)}
	tree := NewTree(pager)
	for i := range 100 {
		key := []byte("key-" + zeroPad(i, 4))
		if err := tree.Put(key, []byte("value")); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
	}

	pager.reads = 0
	var found []string
	if err := tree.ScanPrefix([]byte("key-005"), func(key, value []byte) bool {
		found = append(found, string(key))
		return true
	}); err != nil {
		t.Fatalf("ScanPrefix: %v", err)
	}
	if len(found) != 10 {
		t.Fatalf("ScanPrefix: got %d keys want 10: %v", len(found), found)
	}
	if pager.reads >= int(pager.PageCount()) {
		t.Fatalf("ScanPrefix read %d pages, want fewer than full tree page count %d", pager.reads, pager.PageCount())
	}
}

func TestTreeSnapshotIsolationOnPutAndDelete(t *testing.T) {
	pager := NewMemPager(DefaultPageSize)
	tree := NewTree(pager)

	if err := tree.Put([]byte("key"), []byte("v1")); err != nil {
		t.Fatalf("Put(v1): %v", err)
	}
	rootV1 := tree.RootID()

	if err := tree.Put([]byte("key"), []byte("v2")); err != nil {
		t.Fatalf("Put(v2): %v", err)
	}
	snapV1 := OpenTree(pager, rootV1)

	val, found, err := snapV1.Get([]byte("key"))
	if err != nil {
		t.Fatalf("snap Get(v1): %v", err)
	}
	if !found || string(val) != "v1" {
		t.Fatalf("snap Get(v1): found=%v val=%q", found, val)
	}

	val, found, err = tree.Get([]byte("key"))
	if err != nil {
		t.Fatalf("live Get(v2): %v", err)
	}
	if !found || string(val) != "v2" {
		t.Fatalf("live Get(v2): found=%v val=%q", found, val)
	}

	rootV2 := tree.RootID()
	if _, err := tree.Delete([]byte("key")); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	snapV2 := OpenTree(pager, rootV2)

	val, found, err = snapV2.Get([]byte("key"))
	if err != nil {
		t.Fatalf("snap Get(v2): %v", err)
	}
	if !found || string(val) != "v2" {
		t.Fatalf("snap Get(v2): found=%v val=%q", found, val)
	}

	_, found, err = tree.Get([]byte("key"))
	if err != nil {
		t.Fatalf("live Get(after delete): %v", err)
	}
	if found {
		t.Fatal("live tree should not find deleted key")
	}
}

func TestSuperblockRoundTrip(t *testing.T) {
	sb := Superblock{
		Magic:        SuperblockMagic,
		Version:      1,
		Generation:   42,
		RootPage:     10,
		FreelistPage: 20,
		PageCount:    100,
	}
	var buf [SuperblockSize]byte
	EncodeSuperblock(buf[:], &sb)

	sb2, err := DecodeSuperblock(buf[:])
	if err != nil {
		t.Fatalf("DecodeSuperblock: %v", err)
	}
	if sb2.Generation != 42 || sb2.RootPage != 10 || sb2.FreelistPage != 20 || sb2.PageCount != 100 {
		t.Errorf("mismatch: gen=%d root=%d free=%d count=%d",
			sb2.Generation, sb2.RootPage, sb2.FreelistPage, sb2.PageCount)
	}
}

func TestPickSuperblock(t *testing.T) {
	a := Superblock{Magic: SuperblockMagic, Version: 1, Generation: 5, RootPage: 1}
	b := Superblock{Magic: SuperblockMagic, Version: 1, Generation: 10, RootPage: 2}

	var abuf, bbuf [SuperblockSize]byte
	EncodeSuperblock(abuf[:], &a)
	EncodeSuperblock(bbuf[:], &b)

	result := PickSuperblock(abuf[:], bbuf[:])
	if result == nil || result.Generation != 10 {
		t.Fatal("should pick higher generation")
	}

	result = PickSuperblock([]byte("bad"), bbuf[:])
	if result == nil || result.Generation != 10 {
		t.Fatal("should pick b when a is corrupt")
	}
}

func zeroPad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

type countingPager struct {
	Pager
	reads int
}

func (p *countingPager) ReadPage(id PageID, buf []byte) error {
	p.reads++
	return p.Pager.ReadPage(id, buf)
}
