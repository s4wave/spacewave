package segment

import (
	"bytes"
	"strconv"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	w := NewWriter()
	w.Add([]byte("charlie"), []byte("value3"))
	w.Add([]byte("alpha"), []byte("value1"))
	w.Add([]byte("bravo"), []byte("value2"))

	var buf bytes.Buffer
	written, err := w.Build(&buf)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if written != int64(buf.Len()) {
		t.Fatalf("written=%d but buf.Len()=%d", written, buf.Len())
	}

	data := buf.Bytes()
	rd, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	if rd.EntryCount() != 3 {
		t.Fatalf("entry count: got %d, want 3", rd.EntryCount())
	}
	if string(rd.MinKey()) != "alpha" {
		t.Fatalf("min key: got %q, want %q", rd.MinKey(), "alpha")
	}
	if string(rd.MaxKey()) != "charlie" {
		t.Fatalf("max key: got %q, want %q", rd.MaxKey(), "charlie")
	}

	entries, err := rd.ReadEntries()
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries: got %d, want 3", len(entries))
	}

	// Entries must be sorted by key.
	want := []struct {
		key, val string
	}{
		{"alpha", "value1"},
		{"bravo", "value2"},
		{"charlie", "value3"},
	}
	for i, w := range want {
		if string(entries[i].Key) != w.key {
			t.Errorf("entry %d key: got %q, want %q", i, entries[i].Key, w.key)
		}
		if string(entries[i].Value) != w.val {
			t.Errorf("entry %d value: got %q, want %q", i, entries[i].Value, w.val)
		}
		if entries[i].Tombstone {
			t.Errorf("entry %d: unexpected tombstone", i)
		}
	}
}

func TestGet(t *testing.T) {
	w := NewWriter()
	w.Add([]byte("bar"), []byte("bval"))
	w.Add([]byte("foo"), []byte("fval"))

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data := buf.Bytes()
	rd, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	val, ok, err := rd.Get([]byte("foo"))
	if err != nil {
		t.Fatalf("Get(foo): %v", err)
	}
	if !ok {
		t.Fatal("Get(foo): not found")
	}
	if string(val) != "fval" {
		t.Fatalf("Get(foo): got %q, want %q", val, "fval")
	}

	_, ok, err = rd.Get([]byte("missing"))
	if err != nil {
		t.Fatalf("Get(missing): %v", err)
	}
	if ok {
		t.Fatal("Get(missing): should not be found")
	}

	_, ok, err = rd.Get([]byte("aaa"))
	if err != nil {
		t.Fatalf("Get(aaa): %v", err)
	}
	if ok {
		t.Fatal("Get(aaa): should not be found (below min)")
	}

	_, ok, err = rd.Get([]byte("zzz"))
	if err != nil {
		t.Fatalf("Get(zzz): %v", err)
	}
	if ok {
		t.Fatal("Get(zzz): should not be found (above max)")
	}
}

func TestEmptyWriter(t *testing.T) {
	w := NewWriter()
	var buf bytes.Buffer
	_, err := w.Build(&buf)
	if err == nil {
		t.Fatal("expected error for empty writer")
	}
}

func TestCRC32Corruption(t *testing.T) {
	w := NewWriter()
	w.Add([]byte("key"), []byte("val"))

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data := buf.Bytes()
	// Corrupt a byte in the data block.
	data[HeaderSize+10] ^= 0xFF

	_, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Fatal("expected CRC32 error")
	}
}

func TestSparseIndex1K(t *testing.T) {
	w := NewWriter()
	w.SetIndexInterval(16)

	// Add 1000 entries with zero-padded keys for proper sort order.
	for i := 0; i < 1000; i++ {
		key := "key-" + zeroPad(i, 4)
		val := "val-" + strconv.Itoa(i)
		w.Add([]byte(key), []byte(val))
	}

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data := buf.Bytes()
	rd, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	if rd.EntryCount() != 1000 {
		t.Fatalf("entry count: got %d, want 1000", rd.EntryCount())
	}

	// Verify sparse index was built.
	idx := rd.Index()
	// With 1000 entries and interval 16, expect ceil(1000/16) = 63 index entries.
	expectedIdx := (1000 + 15) / 16
	if len(idx) != expectedIdx {
		t.Fatalf("index entries: got %d, want %d", len(idx), expectedIdx)
	}

	// Point lookup: first, middle, last, missing.
	cases := []struct {
		key   string
		val   string
		found bool
	}{
		{"key-0000", "val-0", true},
		{"key-0500", "val-500", true},
		{"key-0999", "val-999", true},
		{"key-1000", "", false},
		{"aaa", "", false},
		{"zzz", "", false},
	}
	for _, tc := range cases {
		val, ok, err := rd.Get([]byte(tc.key))
		if err != nil {
			t.Errorf("Get(%s): %v", tc.key, err)
			continue
		}
		if ok != tc.found {
			t.Errorf("Get(%s): found=%v, want %v", tc.key, ok, tc.found)
			continue
		}
		if ok && string(val) != tc.val {
			t.Errorf("Get(%s): got %q, want %q", tc.key, val, tc.val)
		}
	}
}

func TestBloomFilter(t *testing.T) {
	n := 10000
	fpr := 0.001 // 0.1%

	bf := NewBloomFilter(n, fpr)

	// Insert n keys.
	for i := 0; i < n; i++ {
		bf.Add([]byte("bloom-" + zeroPad(i, 5)))
	}

	// All inserted keys must be found.
	for i := 0; i < n; i++ {
		if !bf.MayContain([]byte("bloom-" + zeroPad(i, 5))) {
			t.Fatalf("false negative at i=%d", i)
		}
	}

	// Test false-positive rate with non-inserted keys.
	fp := 0
	tests := 100000
	for i := 0; i < tests; i++ {
		key := []byte("nope-" + zeroPad(i, 6))
		if bf.MayContain(key) {
			fp++
		}
	}

	observedFPR := float64(fp) / float64(tests)
	// Allow up to 5x the target FPR to account for randomness.
	maxAllowed := fpr * 5
	if observedFPR > maxAllowed {
		t.Fatalf("FPR too high: observed %.4f, target %.4f, max %.4f", observedFPR, fpr, maxAllowed)
	}
	t.Logf("bloom FPR: target=%.4f observed=%.4f (%d/%d)", fpr, observedFPR, fp, tests)
}

func TestBloomInSSTable(t *testing.T) {
	w := NewWriter()
	for i := 0; i < 100; i++ {
		w.Add([]byte("key-"+zeroPad(i, 3)), []byte("val"))
	}

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data := buf.Bytes()
	rd, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	if rd.Bloom() == nil {
		t.Fatal("bloom filter not loaded")
	}
	if rd.Header().BloomSize == 0 {
		t.Fatal("bloom size is 0")
	}

	// Existing keys must be found via Get.
	val, ok, err := rd.Get([]byte("key-050"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get(key-050): not found")
	}
	if string(val) != "val" {
		t.Fatalf("Get(key-050): got %q", val)
	}

	// Non-existing key should not be found.
	_, ok, err = rd.Get([]byte("key-999"))
	if err != nil {
		t.Fatalf("Get(key-999): %v", err)
	}
	if ok {
		t.Fatal("Get(key-999): should not be found")
	}
}

func TestTombstones(t *testing.T) {
	w := NewWriter()
	w.Add([]byte("alive"), []byte("value"))
	w.AddTombstone([]byte("dead"))
	w.Add([]byte("ghost"), []byte("boo"))

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data := buf.Bytes()
	rd, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	entries, err := rd.ReadEntries()
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries: got %d, want 3", len(entries))
	}

	// Sorted order: alive, dead, ghost
	if string(entries[0].Key) != "alive" || entries[0].Tombstone {
		t.Errorf("entry 0: got key=%q tombstone=%v", entries[0].Key, entries[0].Tombstone)
	}
	if string(entries[1].Key) != "dead" || !entries[1].Tombstone {
		t.Errorf("entry 1: got key=%q tombstone=%v, want dead/true", entries[1].Key, entries[1].Tombstone)
	}
	if string(entries[2].Key) != "ghost" || entries[2].Tombstone {
		t.Errorf("entry 2: got key=%q tombstone=%v", entries[2].Key, entries[2].Tombstone)
	}

	// Get on tombstoned key should return not found.
	_, ok, err := rd.Get([]byte("dead"))
	if err != nil {
		t.Fatalf("Get(dead): %v", err)
	}
	if ok {
		t.Fatal("Get(dead): should not be found (tombstoned)")
	}

	// Get on alive key should work.
	val, ok, err := rd.Get([]byte("alive"))
	if err != nil {
		t.Fatalf("Get(alive): %v", err)
	}
	if !ok {
		t.Fatal("Get(alive): not found")
	}
	if string(val) != "value" {
		t.Fatalf("Get(alive): got %q", val)
	}
}

func zeroPad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func TestHeaderEncodeDecode(t *testing.T) {
	h := Header{
		Magic:       Magic,
		Version:     CurrentVersion,
		Flags:       0,
		EntryCount:  42,
		DataOffset:  100,
		DataSize:    200,
		IndexOffset: 300,
		IndexSize:   50,
		BloomOffset: 350,
		BloomSize:   25,
		MinKeySize:  3,
		MaxKeySize:  10,
	}

	var buf [HeaderSize]byte
	h.Encode(buf[:])

	h2, err := DecodeHeader(buf[:])
	if err != nil {
		t.Fatalf("DecodeHeader: %v", err)
	}

	if h2.EntryCount != 42 {
		t.Errorf("EntryCount: got %d, want 42", h2.EntryCount)
	}
	if h2.DataOffset != 100 {
		t.Errorf("DataOffset: got %d, want 100", h2.DataOffset)
	}
	if h2.IndexSize != 50 {
		t.Errorf("IndexSize: got %d, want 50", h2.IndexSize)
	}
	if h2.BloomSize != 25 {
		t.Errorf("BloomSize: got %d, want 25", h2.BloomSize)
	}
	if h2.MinKeySize != 3 {
		t.Errorf("MinKeySize: got %d, want 3", h2.MinKeySize)
	}
}
