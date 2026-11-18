//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

func testKey(data []byte) []byte {
	sum := sha256.Sum256(data)
	return append(append([]byte{}, keyPrefix...), sum[:]...)
}

func encRefs(t *testing.T, refs []*block.BlockRef) []byte {
	var out []byte
	for _, r := range refs {
		b, err := r.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		out = binary.AppendUvarint(out, uint64(len(b)))
		out = append(out, b...)
	}
	if out == nil {
		out = []byte{}
	}
	return out
}

// TestWriteResume writes two batches, cuts the files as a crash mid-batch
// would leave them, resumes, and checks the frames decode and verify.
func TestWriteResume(t *testing.T) {
	dir := t.TempDir()
	d := [][]byte{[]byte("a"), []byte("bb"), []byte("ccc"), []byte("dddd")}
	keys := make([][]byte, len(d))
	for i := range d {
		keys[i] = testKey(d[i])
	}
	child := &block.BlockRef{Hash: hash.NewHash(hash.HashType_HashType_SHA256, sha256.New().Sum(nil))}
	rows := map[string]row{
		string(keys[0]): {data: d[0], refs: encRefs(t, []*block.BlockRef{child}), known: true},
		string(keys[1]): {data: d[1], refs: nil, known: false},
		string(keys[3]): {data: []byte("wrong"), refs: []byte{}, known: true},
	}

	o, err := openOutput(dir, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := o.write(0, keys[:2], rows); err != nil {
		t.Fatal(err)
	}
	// A second batch whose ledger line never lands: frames written, no line.
	if _, err := o.blocks.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	o.close()

	o, err = openOutput(dir, "t")
	if err != nil {
		t.Fatal(err)
	}
	if o.next != 1 || o.found != 2 || o.missed != 0 {
		t.Fatalf("resume next %d found %d missed %d", o.next, o.found, o.missed)
	}
	if err := o.write(1, keys[2:], rows); err != nil {
		t.Fatal(err)
	}
	o.close()

	misses, _ := os.ReadFile(filepath.Join(dir, "t.misses"))
	bad, _ := os.ReadFile(filepath.Join(dir, "t.hashbad"))
	if bytes.Count(misses, []byte("\n")) != 1 || bytes.Count(bad, []byte("\n")) != 1 {
		t.Fatalf("misses %q bad %q", misses, bad)
	}

	// Decode the frames as block-export frames.
	b, _ := os.ReadFile(filepath.Join(dir, "t.blocks"))
	var n int
	for len(b) > 0 {
		kl, w := binary.Uvarint(b)
		b = b[w:]
		key := b[:kl]
		b = b[kl:]
		known, w := binary.Uvarint(b)
		b = b[w:]
		ol, w := binary.Uvarint(b)
		b = b[w:]
		stored, err := block.DecodeBlockObject(b[:ol])
		if err != nil {
			t.Fatal(err)
		}
		b = b[ol:]
		sum := sha256.Sum256(stored.Data)
		if !bytes.Equal(sum[:], key[4:]) {
			t.Fatalf("frame %d hash", n)
		}
		if (known == 1) != stored.RefsKnown {
			t.Fatalf("frame %d known %d refsKnown %v", n, known, stored.RefsKnown)
		}
		if n == 0 && (len(stored.Refs) != 1 || !stored.Refs[0].EqualVT(child)) {
			t.Fatalf("frame 0 refs %v", stored.Refs)
		}
		n++
	}
	if n != 2 {
		t.Fatalf("frames %d", n)
	}
}

// TestReadEnvNoEcho checks a malformed env file's error names no text.
func TestReadEnvNoEcho(t *testing.T) {
	p := filepath.Join(t.TempDir(), "e")
	if err := os.WriteFile(p, []byte("HYPERLINE_WORLD_SQL_HOST=h\nSECRETVALUE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readEnv(p)
	if err == nil || bytes.Contains([]byte(err.Error()), []byte("SECRET")) {
		t.Fatalf("err %v", err)
	}
}

// fakePages serves the sorted keys a page at a time, as the keyset query
// does, counting the queries and failing the query numbered failAt.
type fakePages struct {
	// keys is the store's keys in order.
	keys [][]byte
	// calls counts the queries served.
	calls int
	// failAt is the query number that fails, 0 for none.
	failAt int
	// slowAt is the query number reported as slow, 0 for none.
	slowAt int
}

func (f *fakePages) page(_ context.Context, after []byte, n int) ([][]byte, time.Duration, error) {
	// Fail or slow the chosen query.
	f.calls++
	if f.calls == f.failAt {
		return nil, 0, errors.New("server gone")
	}
	took := time.Millisecond
	if f.calls == f.slowAt {
		took = time.Minute
	}

	// Return the next n keys above after.
	i := sort.Search(len(f.keys), func(i int) bool { return bytes.Compare(f.keys[i], after) > 0 })
	return f.keys[i:min(i+n, len(f.keys))], took, nil
}

// TestListKeysResume lists 25 keys in pages of 10, stopped by an error after
// two pages and with a torn ledger line, then resumes, and checks the list
// holds each key once in order, that a slow query stops the listing after
// recording its page, and that a done listing makes no query.
func TestListKeysResume(t *testing.T) {
	// Build 25 keys in order.
	dir := t.TempDir()
	var keys [][]byte
	for i := range 25 {
		keys = append(keys, testKey([]byte{byte(i)}))
	}
	slices.SortFunc(keys, bytes.Compare)

	// Stop at the third query, then tear the ledger's end.
	f := &fakePages{keys: keys, failAt: 3}
	o, err := openKeyOutput(dir, "k")
	if err != nil {
		t.Fatal(err)
	}
	if code, err := listKeys(t.Context(), f.page, o, 10, 0, time.Second, ""); code != exitStop || err == nil {
		t.Fatalf("first run code %d err %v", code, err)
	}
	if _, err := o.ledger.WriteString("page 2 keys 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := o.list.WriteString("torn\n"); err != nil {
		t.Fatal(err)
	}
	o.close()

	// Resume with a slow third page: it is recorded, then the run stops.
	f = &fakePages{keys: keys, slowAt: 1}
	o, err = openKeyOutput(dir, "k")
	if err != nil {
		t.Fatal(err)
	}
	if o.pages != 2 || o.keys != 20 || o.end {
		t.Fatalf("resume pages %d keys %d end %v", o.pages, o.keys, o.end)
	}
	if code, err := listKeys(t.Context(), f.page, o, 10, 0, time.Second, ""); code != exitStop || err == nil {
		t.Fatalf("slow run code %d err %v", code, err)
	}
	o.close()

	// Reopen the finished listing: it is done and queries nothing.
	f = &fakePages{keys: keys}
	o, err = openKeyOutput(dir, "k")
	if err != nil {
		t.Fatal(err)
	}
	if code, err := listKeys(t.Context(), f.page, o, 10, 0, time.Second, ""); code != 0 || err != nil || f.calls != 0 {
		t.Fatalf("done run code %d err %v calls %d", code, err, f.calls)
	}
	o.close()

	// The list holds every key once, in order.
	data, _ := os.ReadFile(filepath.Join(dir, "k.keys"))
	var want strings.Builder
	for _, k := range keys {
		want.WriteString(hex.EncodeToString(k) + "\n")
	}
	if string(data) != want.String() {
		t.Fatalf("list %d bytes, want %d", len(data), want.Len())
	}
}

// TestListKeysRate checks the listing waits the interval between queries.
func TestListKeysRate(t *testing.T) {
	// List 3 pages at 20 queries a second.
	var keys [][]byte
	for i := range 5 {
		keys = append(keys, testKey([]byte{byte(i)}))
	}
	slices.SortFunc(keys, bytes.Compare)
	o, err := openKeyOutput(t.TempDir(), "k")
	if err != nil {
		t.Fatal(err)
	}
	defer o.close()

	// Three queries take at least the two intervals between them.
	f := &fakePages{keys: keys}
	start := time.Now()
	if code, err := listKeys(t.Context(), f.page, o, 2, 50*time.Millisecond, time.Second, ""); code != 0 || err != nil {
		t.Fatalf("code %d err %v", code, err)
	}
	if f.calls != 3 || time.Since(start) < 100*time.Millisecond || o.keys != 5 {
		t.Fatalf("calls %d keys %d in %v", f.calls, o.keys, time.Since(start))
	}
}
