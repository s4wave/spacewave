package store

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// indexPackDirEnv names a directory of real packfiles to compare in
// TestTailIndexMatchesParsedIndexOnPackDir.
const indexPackDirEnv = "PACKFILE_INDEX_PACK_DIR"

// buildBothIndexes builds the parsed and in-place index of a pack tail with
// the reader the store uses, returning each build's error.
func buildBothIndexes(tail []byte, size int64, blockCount uint64) (*parsedIndex, *tailIndex, error, error) {
	e := NewPackReader("test-pack", size, nil)
	defer e.Close()
	e.SetExpectedBlockCount(blockCount)
	entries, perr := e.parseIndexTail(tail)
	var p *parsedIndex
	if perr == nil {
		p = newParsedIndex(entries)
	}
	t, terr := newTailIndex(bytes.Clone(tail), uint64(size), blockCount)
	return p, t, perr, terr
}

// absentKeys derives keys near each key of an index: one byte changed at each
// end, truncated, extended, and the empty and oversize keys.
func absentKeys(keys [][]byte) [][]byte {
	out := [][]byte{nil, {}, {0}, bytes.Repeat([]byte{0xff}, 64)}
	for _, k := range keys {
		last := bytes.Clone(k)
		last[len(last)-1] ^= 1
		first := bytes.Clone(k)
		first[0] ^= 1
		out = append(out,
			last, first,
			bytes.Clone(k[:len(k)-1]),
			append(bytes.Clone(k), 0),
			append(bytes.Clone(k), k...),
			bytes.Clone(k[:len(k)/2]),
		)
	}
	return out
}

// requireSameIndex fails the test unless the in-place index answers every
// probe exactly as the parsed index does: every key, near-miss keys, the
// offset order, and a seek at and around every extent boundary.
func requireSameIndex(t *testing.T, p *parsedIndex, ti *tailIndex, size int64) {
	t.Helper()
	if p.len() != ti.len() {
		t.Fatalf("len: parsed %d, in place %d", p.len(), ti.len())
	}

	// Every present key finds the same extent, and near-miss keys agree.
	var keys [][]byte
	for _, e := range p.byKey {
		keys = append(keys, e.GetKey())
	}
	probes := slices.Clone(keys)
	probes = append(probes, absentKeys(keys)...)
	for _, k := range probes {
		pe, pok := p.find(k)
		te, tok := ti.find(k)
		if pe != te || pok != tok {
			t.Fatalf("find(%x): parsed (%v, %v), in place (%v, %v)", k, pe, pok, te, tok)
		}
	}
	for _, k := range keys {
		if _, ok := ti.find(k); !ok {
			t.Fatalf("find(%x): present key not found in place", k)
		}
	}

	// The offset order lists the same keys and extents.
	seeks := []int64{-1, 0, 1, size, size + 1}
	for pos := range p.len() {
		pk, pe := p.atOffset(pos)
		tk, te := ti.atOffset(pos)
		if !bytes.Equal(pk, tk) || pe != te {
			t.Fatalf("atOffset(%d): parsed (%x, %v), in place (%x, %v)", pos, pk, pe, tk, te)
		}
		seeks = append(seeks, pe.off-1, pe.off, pe.off+1, pe.end-1, pe.end, pe.end+1)
	}
	for _, s := range seeks {
		if a, b := p.seekEnd(s), ti.seekEnd(s); a != b {
			t.Fatalf("seekEnd(%d): parsed %d, in place %d", s, a, b)
		}
		if a, b := p.seekOffset(s), ti.seekOffset(s); a != b {
			t.Fatalf("seekOffset(%d): parsed %d, in place %d", s, a, b)
		}
	}
}

// requireSameVerdict builds both indexes from tail, which must fail or succeed
// in both, and compares them when they build.
func requireSameVerdict(t *testing.T, tail []byte, size int64, blockCount uint64, what string) bool {
	t.Helper()
	p, ti, perr, terr := buildBothIndexes(tail, size, blockCount)
	if (perr == nil) != (terr == nil) {
		t.Fatalf("%s: parsed err %v, in place err %v", what, perr, terr)
	}
	if perr != nil {
		return false
	}
	requireSameIndex(t, p, ti, size)
	return true
}

// craftedPack builds a pack of payload bytes followed by an index of entries
// written in the order given, which kvfile.WriteIndex would sort and reject
// if duplicated, and returns its bytes. Entries may share offsets.
func craftedPack(t *testing.T, payload int, entries []*kvfile.IndexEntry) []byte {
	t.Helper()
	buf := bytes.NewBuffer(make([]byte, payload))
	var poss []uint64
	for _, e := range entries {
		raw, err := e.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(raw)
		poss = append(poss, uint64(buf.Len()))
		buf.Write(protobuf_go_lite.AppendVarint(nil, uint64(len(raw))))
	}
	poss = append(poss, uint64(len(entries)))
	for _, pos := range poss {
		buf.Write(binary.LittleEndian.AppendUint64(nil, pos))
	}
	return buf.Bytes()
}

// sortedEntries sorts entries by key, as a valid index lists them.
func sortedEntries(entries []*kvfile.IndexEntry) []*kvfile.IndexEntry {
	slices.SortStableFunc(entries, func(a, b *kvfile.IndexEntry) int { return bytes.Compare(a.Key, b.Key) })
	return entries
}

// testBlockHash returns the hash key of block i.
func testBlockHash(t *testing.T, hashType hash.HashType, i int) []byte {
	t.Helper()
	h, err := hash.Sum(hashType, fmt.Appendf(nil, "block %d", i))
	if err != nil {
		t.Fatal(err)
	}
	return packfile.BlockKey(h)
}

// TestTailIndexMatchesParsedIndex compares the in-place index with the parsed
// index on built and crafted packs, and on corrupt and truncated tails, which
// must fail in both.
func TestTailIndexMatchesParsedIndex(t *testing.T) {
	// Real packs of many sizes, with blocks of mixed lengths so the payload
	// order differs from the key order.
	for _, n := range []int{1, 2, 3, 17, 300, 4096} {
		var items []packItem
		for i := range n {
			items = append(items, testPackItem(t, fmt.Sprintf("block %d %s", i, bytes.Repeat([]byte{'x'}, (i*7919)%509))))
		}
		data, _ := packItems(t, items)
		tail := mustReadIndexTail(t, data)
		if !requireSameVerdict(t, tail, int64(len(data)), uint64(n), fmt.Sprintf("pack of %d", n)) {
			t.Fatalf("pack of %d: tail rejected", n)
		}
	}

	// Crafted packs: offsets that tie, empty values, a single entry, and every
	// supported hash type.
	for _, hashType := range []hash.HashType{hash.HashType_HashType_SHA256, hash.HashType_HashType_BLAKE3} {
		var entries []*kvfile.IndexEntry
		offs := []uint64{40, 10, 10, 10, 0, 90, 40, 0}
		sizes := []uint64{10, 0, 0, 5, 10, 10, 0, 0}
		for i := range offs {
			entries = append(entries, &kvfile.IndexEntry{Key: testBlockHash(t, hashType, i), Offset: offs[i], Size: sizes[i]})
		}
		data := craftedPack(t, 100, sortedEntries(entries))
		tail := mustReadIndexTail(t, data)
		if !requireSameVerdict(t, tail, int64(len(data)), uint64(len(entries)), "crafted ties") {
			t.Fatal("crafted ties: tail rejected")
		}
	}

	// A tail corrupted in any byte, or cut at any point, or checked against a
	// different size or block count, fails in both or builds the same index.
	var items []packItem
	for i := range 6 {
		items = append(items, testPackItem(t, fmt.Sprintf("corrupt %d", i)))
	}
	data, _ := packItems(t, items)
	size := int64(len(data))
	tail := mustReadIndexTail(t, data)
	var built, failed int
	count := func(ok bool) {
		if ok {
			built++
		} else {
			failed++
		}
	}
	for i := range tail {
		for _, mut := range []func(byte) byte{func(b byte) byte { return b ^ 0xff }, func(b byte) byte { return b ^ 1 }, func(byte) byte { return 0 }} {
			c := bytes.Clone(tail)
			c[i] = mut(c[i])
			count(requireSameVerdict(t, c, size, 6, fmt.Sprintf("byte %d", i)))
		}
	}
	for i := range len(tail) + 1 {
		count(requireSameVerdict(t, tail[:i], size, 6, fmt.Sprintf("cut to %d", i)))
		count(requireSameVerdict(t, tail[i:], size, 6, fmt.Sprintf("cut from %d", i)))
	}
	for _, d := range []int64{-9, -1, 0, 1, 9} {
		for _, bc := range []uint64{0, 5, 6, 7} {
			count(requireSameVerdict(t, tail, size+d, bc, fmt.Sprintf("size %+d count %d", d, bc)))
		}
	}
	if built == 0 || failed == 0 {
		t.Fatalf("corruption sweep built %d and failed %d tails, want both", built, failed)
	}
}

// TestTailIndexRejectsMalformedEntries checks the entry rules the parsed index
// enforces: unsorted or duplicate keys, an empty or invalid key, and an
// extent beyond the tail all fail in both, and an empty pack builds in both.
func TestTailIndexRejectsMalformedEntries(t *testing.T) {
	k0, k1 := testBlockHash(t, hash.HashType_HashType_SHA256, 0), testBlockHash(t, hash.HashType_HashType_SHA256, 1)
	if bytes.Compare(k0, k1) > 0 {
		k0, k1 = k1, k0
	}
	for name, entries := range map[string][]*kvfile.IndexEntry{
		"duplicate": {{Key: k0, Offset: 0, Size: 5}, {Key: k0, Offset: 5, Size: 5}},
		"empty key": {{Key: nil, Offset: 0, Size: 5}},
		"bad key":   {{Key: []byte("not a hash"), Offset: 0, Size: 5}},
		"past tail": {{Key: k0, Offset: 90, Size: 11}},
		"overflow":  {{Key: k0, Offset: 90, Size: ^uint64(0)}},
		"unsorted":  {{Key: k1, Offset: 0, Size: 5}, {Key: k0, Offset: 5, Size: 5}},
		"valid":     sortedEntries([]*kvfile.IndexEntry{{Key: k0, Offset: 0, Size: 5}, {Key: k1, Offset: 5, Size: 5}}),
	} {
		data := craftedPack(t, 100, entries)
		tail := mustReadIndexTail(t, data)
		ok := requireSameVerdict(t, tail, int64(len(data)), uint64(len(entries)), name)
		if ok != (name == "valid") {
			t.Fatalf("%s: built = %v", name, ok)
		}
	}
	// An empty pack has an empty tail, and both indexes accept it as empty.
	for _, tail := range [][]byte{nil, {}} {
		if !requireSameVerdict(t, tail, 0, 0, "empty pack") {
			t.Fatal("empty pack: rejected")
		}
	}
}

// TestTailIndexMatchesParsedIndexOnPackDir compares both indexes on every
// packfile in the directory named by PACKFILE_INDEX_PACK_DIR.
func TestTailIndexMatchesParsedIndexOnPackDir(t *testing.T) {
	dir := os.Getenv(indexPackDirEnv)
	if dir == "" {
		t.Skipf("%s is not set", indexPackDirEnv)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no packs in %s: %v", dir, err)
	}
	var keys int
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		fi, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		_, tail, err := kvfile.ReadIndexTail(f, uint64(fi.Size()))
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		rd, err := kvfile.BuildReaderWithIndexTail(tail, uint64(fi.Size()))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !requireSameVerdict(t, tail, fi.Size(), rd.Size(), name) {
			t.Fatalf("%s: tail rejected", name)
		}
		keys += int(rd.Size())
	}
	t.Logf("compared %d packs, %d keys", len(files), keys)
}
