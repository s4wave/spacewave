package packahead

import (
	"bytes"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// sectorBytes is the unit a crash keeps or loses.
const sectorBytes = 512

// crashCase is one state a crash may leave a pack's last batch in.
type crashCase struct {
	// name describes the state.
	name string
	// mutate rewrites the pack file's bytes. last spans the last batch.
	mutate func(data []byte, start, end int) []byte
	// keepsLast reports whether recovery keeps the last batch.
	keepsLast bool
}

// TestPackAheadExclusiveOpen keeps a second opener out of recovery until the
// first log closes.
func TestPackAheadExclusiveOpen(t *testing.T) {
	// Open a log with an empty pack that recovery would remove.
	ctx, dir := t.Context(), t.TempDir()
	first, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.Close)

	// Require an error naming the directory, with the open pack still intact.
	second, err := Open(ctx, dir, Options{})
	if err == nil {
		second.Close()
		t.Fatal("second Open succeeded while the first log was open")
	}
	if !strings.Contains(err.Error(), strconv.Quote(dir)) || !strings.Contains(err.Error(), "already open") {
		t.Fatalf("second Open returned %v, want an already-open error naming %s", err, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, openName(1))); err != nil {
		t.Fatalf("second Open disturbed the first log's open pack: %v", err)
	}

	// Open the same directory after the first log releases ownership.
	first.Close()
	reopened, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatalf("Open after Close: %v", err)
	}
	reopened.Close()
}

// TestPackAheadCrash leaves the last batch of an open pack in every state a
// crash can and checks that recovery keeps every earlier batch, keeps the
// last batch only when all of it survived, never returns wrong bytes, and
// finalizes a pack a remote reader accepts.
func TestPackAheadCrash(t *testing.T) {
	ctx := t.Context()

	// Write six batches. The first five stand for acknowledged writes and
	// the sixth for a batch written but not synced when the crash came.
	src := t.TempDir()
	l, err := Open(ctx, src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var batches [][]Block
	for i := range 6 {
		batch := []Block{testBlock(t, i*3, 2000+i), testBlock(t, i*3+1, 5000), testBlock(t, i*3+2, 700)}
		if err := l.Append(ctx, batch); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, batch)
	}
	l.Close()

	// Find the pack and the bounds of its last batch.
	name := openName(1)
	data, err := os.ReadFile(filepath.Join(src, name))
	if err != nil {
		t.Fatal(err)
	}
	var start, end int64
	for at := int64(0); at < int64(len(data)); {
		_, next, err := decodeBatch(data, at)
		if err != nil {
			t.Fatalf("written pack does not scan at %d: %v", at, err)
		}
		start, end, at = at, next, next
	}
	recLen := int(start) + recordLen(batches[5])

	// List the crash states.
	cases := []crashCase{
		{name: "intact", mutate: func(d []byte, _, _ int) []byte { return d }, keepsLast: true},
		{name: "garbage after", mutate: func(d []byte, _, _ int) []byte {
			return append(d, bytes.Repeat([]byte{0xa5}, 3000)...)
		}, keepsLast: true},
		{name: "record without values", mutate: func(d []byte, _, e int) []byte {
			clear(d[recLen:e])
			return d
		}},
		{name: "values without record", mutate: func(d []byte, s, _ int) []byte {
			clear(d[s:recLen])
			return d
		}},
		{name: "one value byte flipped", mutate: func(d []byte, _, e int) []byte {
			d[e-1] ^= 1
			return d
		}},
	}
	for cut := int(start) + 1; cut < int(end); cut += 97 {
		cases = append(cases, crashCase{name: "cut", mutate: func(d []byte, _, _ int) []byte { return d[:cut] }})
	}
	rnd := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // a seeded test sequence.
	sectors := (int(end) - int(start) + sectorBytes - 1) / sectorBytes
	for range 40 {
		lost := make([]bool, sectors)
		any := false
		for i := range lost {
			lost[i] = rnd.IntN(3) == 0
			any = any || lost[i]
		}
		cases = append(cases, crashCase{name: "sectors lost", mutate: func(d []byte, s, e int) []byte {
			for i, gone := range lost {
				if gone {
					clear(d[s+i*sectorBytes : min(s+(i+1)*sectorBytes, e)])
				}
			}
			return d
		}, keepsLast: !any})
	}

	// Recover each state and check what it kept.
	for _, c := range cases {
		dir := t.TempDir()
		mutated := c.mutate(bytes.Clone(data), int(start), int(end))
		if err := os.WriteFile(filepath.Join(dir, name), mutated, 0o644); err != nil {
			t.Fatal(err)
		}
		checkRecovered(t, dir, c.name, batches, c.keepsLast)
	}
}

// TestPackAheadStartupRejectsEmbeddedRecord puts a complete record at the
// absolute offset of a block value after an unreadable batch header.
func TestPackAheadStartupRejectsEmbeddedRecord(t *testing.T) {
	// Give the embedded record valid checksums and an exact absolute end.
	prefix := []Block{testBlock(t, 1, 64)}
	embedded := []Block{testBlock(t, 2, 64)}
	damaged := []Block{testBlock(t, 3, recordLen(embedded)+dataLen(embedded))}
	later := []Block{testBlock(t, 4, 64)}
	damagedAt := int64(recordLen(prefix) + dataLen(prefix))
	embeddedAt := damagedAt + int64(recordLen(damaged))
	encodeBatch(damaged[0].Value, embeddedAt, embedded)

	// Write genuine adjacent batches and simulate an unreadable middle record.
	dir := t.TempDir()
	l, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, batch := range [][]Block{prefix, damaged, later} {
		if err := l.Append(t.Context(), batch); err != nil {
			l.Close()
			t.Fatal(err)
		}
	}
	l.Close()
	path := filepath.Join(dir, openName(1))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[damagedAt] = 0
	if _, _, err := decodeBatch(data, embeddedAt); err != nil {
		t.Fatalf("embedded record must pass decoding: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Reopen and check both the local table and the finalized index.
	l, err = Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	for _, key := range [][]byte{damaged[0].Key, embedded[0].Key, later[0].Key} {
		if l.Has(key) {
			t.Fatalf("startup indexed key %x beyond the unreadable record", key)
		}
	}
	infos := waitFinalized(t, l, 1)
	checkPack(t, infos[0], map[string][]byte{string(prefix[0].Key): prefix[0].Value}, Options{})
}

// TestPackAheadCrashFinalize leaves a finalized pack under its open name, as a
// crash after its index but before its rename does, with the index whole or
// cut short, and checks that recovery keeps every batch and finalizes again.
func TestPackAheadCrashFinalize(t *testing.T) {
	ctx := t.Context()

	// Write a pack, finalize it at the next open, and keep its bytes.
	src := t.TempDir()
	l, err := Open(ctx, src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var batches [][]Block
	for i := range 4 {
		batch := []Block{testBlock(t, 100+i*2, 4000), testBlock(t, 101+i*2, 900)}
		if err := l.Append(ctx, batch); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, batch)
	}
	l.Close()
	l, err = Open(ctx, src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	infos, _ := l.GetPacks()
	l.Close()
	if len(infos) != 1 {
		t.Fatalf("%d finalized packs", len(infos))
	}
	data, err := os.ReadFile(infos[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	_, batchesEnd, _ := scanBatches(data)

	// Recover the pack with its index whole and cut at points inside it.
	for cut := int(batchesEnd); cut <= len(data); cut += max(1, (len(data)-int(batchesEnd))/7) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, openName(1)), data[:cut], 0o644); err != nil {
			t.Fatal(err)
		}
		checkRecovered(t, dir, "index cut", batches, true)
	}
}

// checkRecovered opens the Log in dir and checks that every batch but the
// last batch reads back, that the last batch reads back whole when keepsLast
// is true and not at all otherwise, and that the recovered pack is a valid
// packfile.
func checkRecovered(t *testing.T, dir, name string, batches [][]Block, keepsLast bool) {
	t.Helper()
	l, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatalf("%s: open: %v", name, err)
	}
	defer l.Close()

	// Check every block: present with its bytes, or absent.
	want := make(map[string][]byte)
	for i, batch := range batches {
		kept := i < len(batches)-1 || keepsLast
		for _, b := range batch {
			got, ok, err := l.Get(b.Key)
			if err != nil {
				t.Fatalf("%s: get: %v", name, err)
			}
			if ok != kept || (ok && !bytes.Equal(got, b.Value)) {
				t.Fatalf("%s: batch %d block: found %v, want %v, bytes equal %v", name, i, ok, kept, bytes.Equal(got, b.Value))
			}
			if kept {
				want[string(b.Key)] = b.Value
			}
		}
	}

	// The recovered pack is one finalized packfile holding exactly them.
	infos, _ := l.GetPacks()
	if len(infos) != 1 || infos[0].Blocks != len(want) {
		t.Fatalf("%s: %d packs, want one of %d blocks", name, len(infos), len(want))
	}
	checkPack(t, infos[0], want, Options{})
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), packExt) && e.Name() != filepath.Base(infos[0].Path) {
			t.Fatalf("%s: stray pack %s", name, e.Name())
		}
	}
}
