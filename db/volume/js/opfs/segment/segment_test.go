package segment

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
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

func TestV1FixtureRemainsWireCompatible(t *testing.T) {
	const legacyHex = "" +
		"4f53535400010000000000010000004a0000000e000000580000000900000061" +
		"0000000700030003000000000000000000000000000000000000000000000000" +
		"00036b657900036b657900036b65790000000576616c756500036b657900000000" +
		"0b0000000fb73ba0b9601d"
	legacy, err := hex.DecodeString(legacyHex)
	if err != nil {
		t.Fatal(err)
	}

	rd, err := NewReader(bytes.NewReader(legacy), int64(len(legacy)))
	if err != nil {
		t.Fatalf("open legacy v1 segment: %v", err)
	}
	value, found, err := rd.Get([]byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(value) != "value" {
		t.Fatalf("legacy lookup = %q, %v; want value, true", value, found)
	}

	w := NewWriter()
	w.Add([]byte("key"), []byte("value"))
	var current bytes.Buffer
	if _, err := w.Build(&current); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current.Bytes(), legacy) {
		t.Fatalf("current v1 encoding changed:\n got %x\nwant %x", current.Bytes(), legacy)
	}
}

func TestBuildWithMetaMatchesLoadedLookupMeta(t *testing.T) {
	w := NewWriter()
	w.SetIndexInterval(2)
	w.Add([]byte("charlie"), []byte("value3"))
	w.AddTombstone([]byte("bravo"))
	w.Add([]byte("alpha"), []byte("value1"))

	var buf bytes.Buffer
	result, err := w.BuildWithMeta(&buf)
	if err != nil {
		t.Fatalf("BuildWithMeta: %v", err)
	}
	if result.Written != int64(buf.Len()) {
		t.Fatalf("written=%d but buf.Len()=%d", result.Written, buf.Len())
	}
	if result.Lookup == nil {
		t.Fatal("Lookup is nil")
	}

	loaded, err := LoadLookupMeta(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("LoadLookupMeta: %v", err)
	}
	if result.Lookup.Header.EntryCount != loaded.Header.EntryCount {
		t.Fatalf("EntryCount=%d, want %d", result.Lookup.Header.EntryCount, loaded.Header.EntryCount)
	}
	if string(result.Lookup.MinKey) != string(loaded.MinKey) {
		t.Fatalf("MinKey=%q, want %q", result.Lookup.MinKey, loaded.MinKey)
	}
	if string(result.Lookup.MaxKey) != string(loaded.MaxKey) {
		t.Fatalf("MaxKey=%q, want %q", result.Lookup.MaxKey, loaded.MaxKey)
	}
	if len(result.Lookup.Index) != len(loaded.Index) {
		t.Fatalf("Index len=%d, want %d", len(result.Lookup.Index), len(loaded.Index))
	}
	for i := range loaded.Index {
		if string(result.Lookup.Index[i].Key) != string(loaded.Index[i].Key) ||
			result.Lookup.Index[i].DataOffset != loaded.Index[i].DataOffset {
			t.Fatalf("Index[%d]=%+v, want %+v", i, result.Lookup.Index[i], loaded.Index[i])
		}
	}

	val, found, err := result.Lookup.Get(bytes.NewReader(buf.Bytes()), []byte("charlie"))
	if err != nil {
		t.Fatalf("Lookup.Get: %v", err)
	}
	if !found || string(val) != "value3" {
		t.Fatalf("Lookup.Get = %q, %v, want value3, true", val, found)
	}
}

func TestLookupMetaStatStreamsLargeValueWithoutReadingIt(t *testing.T) {
	key := []byte("target")
	value := bytes.Repeat([]byte("v"), maxLookupWindowRead+1)
	w := NewWriter()
	w.Add(key, value)

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}
	data := buf.Bytes()
	meta, err := LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("LoadLookupMeta: %v", err)
	}

	valueStart := int64(meta.Header.DataOffset) + 2 + int64(len(key)) + 4
	guard := guardedReaderAt{
		data:  data,
		start: valueStart,
		end:   valueStart + int64(len(value)),
	}
	stat, err := meta.Stat(guard, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !stat.Found || stat.Tombstone {
		t.Fatalf("Stat found=%v tombstone=%v", stat.Found, stat.Tombstone)
	}
	if stat.ValueSize != int64(len(value)) {
		t.Fatalf("Stat size=%d, want %d", stat.ValueSize, len(value))
	}
}

func TestBuildStreamsDataEntries(t *testing.T) {
	w := NewWriter()
	w.SetIndexInterval(8)
	for i := range 4 {
		w.Add([]byte("key-"+strconv.Itoa(i)), bytes.Repeat([]byte{byte('a' + i)}, 128))
	}

	var dst maxWriteRecorder
	result, err := w.BuildWithMeta(&dst)
	if err != nil {
		t.Fatalf("BuildWithMeta: %v", err)
	}
	if result.Written != int64(dst.Len()) {
		t.Fatalf("written=%d but dst.Len()=%d", result.Written, dst.Len())
	}
	if dst.maxWrite > 128 {
		t.Fatalf("max write size=%d, want <= 128", dst.maxWrite)
	}

	if _, err := NewReader(bytes.NewReader(dst.Bytes()), int64(dst.Len())); err != nil {
		t.Fatalf("NewReader: %v", err)
	}
}

type guardedReaderAt struct {
	data  []byte
	start int64
	end   int64
}

func (r guardedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < r.end && off+int64(len(p)) > r.start {
		return 0, io.ErrUnexpectedEOF
	}
	return bytes.NewReader(r.data).ReadAt(p, off)
}

type maxWriteRecorder struct {
	bytes.Buffer
	maxWrite int
}

func (w *maxWriteRecorder) Write(p []byte) (int, error) {
	w.maxWrite = max(w.maxWrite, len(p))
	return w.Buffer.Write(p)
}

func TestEntryIterator(t *testing.T) {
	w := NewWriter()
	w.Add([]byte("charlie"), []byte("value3"))
	w.AddTombstone([]byte("bravo"))
	w.Add([]byte("alpha"), []byte("value1"))

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data := buf.Bytes()
	meta, err := LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("LoadLookupMeta: %v", err)
	}

	it := NewEntryIterator(bytes.NewReader(data), meta)
	var entries []Entry
	for {
		entry, ok, err := it.Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
		entries = append(entries, entry)
	}

	if len(entries) != 3 {
		t.Fatalf("entries: got %d, want 3", len(entries))
	}
	if string(entries[0].Key) != "alpha" || string(entries[0].Value) != "value1" {
		t.Fatalf("entry 0: %+v", entries[0])
	}
	if string(entries[1].Key) != "bravo" || !entries[1].Tombstone {
		t.Fatalf("entry 1: %+v", entries[1])
	}
	if string(entries[2].Key) != "charlie" || string(entries[2].Value) != "value3" {
		t.Fatalf("entry 2: %+v", entries[2])
	}

	if entry, ok, err := it.Next(); err != nil || ok || entry.Key != nil {
		t.Fatalf("exhausted Next: entry=%+v ok=%v err=%v", entry, ok, err)
	}
}

func TestEntryIteratorBuffersSequentialReads(t *testing.T) {
	w := NewWriter()
	value := bytes.Repeat([]byte("v"), 512)
	const count = 4096
	for i := range count {
		w.Add([]byte("key-"+zeroPad(i, 6)), value)
	}
	var encoded bytes.Buffer
	if _, err := w.Build(&encoded); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	meta, err := LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	source := &countingReaderAt{data: data}
	iterator := NewEntryIterator(source, meta)
	for i := range count {
		entry, ok, err := iterator.Next()
		if err != nil || !ok || string(entry.Key) != "key-"+zeroPad(i, 6) || !bytes.Equal(entry.Value, value) {
			t.Fatalf("entry %d: ok=%v err=%v", i, ok, err)
		}
	}
	if _, ok, err := iterator.Next(); ok || err != nil {
		t.Fatalf("exhaustion: ok=%v err=%v", ok, err)
	}
	if source.reads > 40 {
		t.Fatalf("sequential scan used %d storage reads, want at most 40", source.reads)
	}
}

func TestEntryIteratorRejectsTruncatedValueBeforeAlloc(t *testing.T) {
	w := NewWriter()
	w.Add([]byte("key"), []byte("value"))

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}
	data := append([]byte(nil), buf.Bytes()...)
	meta, err := LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("LoadLookupMeta: %v", err)
	}

	valueLenOff := int(meta.Header.DataOffset) + 2 + len("key")
	binary.BigEndian.PutUint32(data[valueLenOff:valueLenOff+4], 1<<30)

	it := NewEntryIterator(bytes.NewReader(data), meta)
	if entry, ok, err := it.Next(); err != io.ErrUnexpectedEOF || ok || entry.Key != nil {
		t.Fatalf("Next: entry=%+v ok=%v err=%v, want unexpected EOF", entry, ok, err)
	}
}

func TestVerifyChecksumRejectsCorruptContent(t *testing.T) {
	w := NewWriter()
	w.Add([]byte("key"), []byte("value"))

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}
	data := append([]byte(nil), buf.Bytes()...)
	if err := VerifyChecksum(bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("VerifyChecksum clean: %v", err)
	}

	data[int(HeaderSize)+2] ^= 0xff
	if err := VerifyChecksum(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("VerifyChecksum corrupt: expected error")
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

func TestLookupMetaLocateBatchCoalescesWindowReads(t *testing.T) {
	w := NewWriter()
	w.SetIndexInterval(4)
	for i := range 8 {
		key := "key-" + zeroPad(i, 4)
		w.Add([]byte(key), []byte("val-"+zeroPad(i, 4)))
	}

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data := buf.Bytes()
	meta, err := LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("LoadLookupMeta: %v", err)
	}

	reader := &countingReaderAt{data: data}
	results, err := meta.LocateBatch(reader, [][]byte{
		[]byte("key-0001"),
		[]byte("key-0002"),
		[]byte("key-0003"),
	}, false)
	if err != nil {
		t.Fatalf("LocateBatch: %v", err)
	}
	if reader.reads != 1 {
		t.Fatalf("data window reads = %d, want 1", reader.reads)
	}
	for i, result := range results {
		if !result.Found || result.Tombstone || result.Value != nil {
			t.Fatalf("result %d = %#v, want live existence hit without value", i, result)
		}
	}
}

func TestLookupMetaLocateBatchValuesAndTombstones(t *testing.T) {
	w := NewWriter()
	w.SetIndexInterval(4)
	w.Add([]byte("key-0000"), []byte("val-0000"))
	w.AddTombstone([]byte("key-0001"))
	w.Add([]byte("key-0002"), []byte("val-0002"))

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data := buf.Bytes()
	meta, err := LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("LoadLookupMeta: %v", err)
	}

	results, err := meta.LocateBatch(bytes.NewReader(data), [][]byte{
		[]byte("key-0000"),
		[]byte("key-0001"),
		[]byte("key-missing"),
		[]byte("key-0002"),
	}, true)
	if err != nil {
		t.Fatalf("LocateBatch: %v", err)
	}
	if !results[0].Found || string(results[0].Value) != "val-0000" || results[0].Tombstone {
		t.Fatalf("live result = %#v, want key-0000 value", results[0])
	}
	if results[1].Found || !results[1].Tombstone || results[1].Value != nil {
		t.Fatalf("tombstone result = %#v, want tombstone miss", results[1])
	}
	if results[2].Found || results[2].Tombstone || results[2].Value != nil {
		t.Fatalf("missing result = %#v, want miss", results[2])
	}
	if !results[3].Found || string(results[3].Value) != "val-0002" || results[3].Tombstone {
		t.Fatalf("live result = %#v, want key-0002 value", results[3])
	}
}

func TestLookupMetaLocateStreamsLargeDataWindow(t *testing.T) {
	const valueSize = 128 * 1024

	w := NewWriter()
	w.SetIndexInterval(16)
	for i := range 17 {
		w.Add(
			[]byte("key-"+zeroPad(i, 4)),
			bytes.Repeat([]byte{byte('a' + i%26)}, valueSize),
		)
	}

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data := buf.Bytes()
	meta, err := LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("LoadLookupMeta: %v", err)
	}

	reader := &recordingReaderAt{data: data}
	val, found, err := meta.Get(reader, []byte("key-0015"))
	if err != nil {
		t.Fatalf("LookupMeta.Get: %v", err)
	}
	if !found {
		t.Fatal("LookupMeta.Get: not found")
	}
	if len(val) != valueSize || val[0] != 'p' || val[len(val)-1] != 'p' {
		t.Fatalf("LookupMeta.Get value mismatch: len=%d first=%q last=%q", len(val), val[0], val[len(val)-1])
	}
	if reader.maxRead > valueSize {
		t.Fatalf("largest ReadAt = %d, want bounded by returned value size %d", reader.maxRead, valueSize)
	}
}

func TestLookupMetaLocateBatchStreamsLargeDataWindowWithoutValues(t *testing.T) {
	const valueSize = 128 * 1024

	w := NewWriter()
	w.SetIndexInterval(16)
	for i := range 17 {
		w.Add(
			[]byte("key-"+zeroPad(i, 4)),
			bytes.Repeat([]byte{byte('a' + i%26)}, valueSize),
		)
	}

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data := buf.Bytes()
	meta, err := LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("LoadLookupMeta: %v", err)
	}

	reader := &recordingReaderAt{data: data}
	results, err := meta.LocateBatch(reader, [][]byte{
		[]byte("key-0001"),
		[]byte("key-0015"),
	}, false)
	if err != nil {
		t.Fatalf("LocateBatch: %v", err)
	}
	for i, result := range results {
		if !result.Found || result.Tombstone || result.Value != nil {
			t.Fatalf("result %d = %#v, want live existence hit without value", i, result)
		}
	}
	if reader.maxRead > len("key-0000") {
		t.Fatalf("largest ReadAt = %d, want key-sized scan without reading values", reader.maxRead)
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
	for i := range 1000 {
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
	for i := range n {
		bf.Add([]byte("bloom-" + zeroPad(i, 5)))
	}

	// All inserted keys must be found.
	for i := range n {
		if !bf.MayContain([]byte("bloom-" + zeroPad(i, 5))) {
			t.Fatalf("false negative at i=%d", i)
		}
	}

	// Test false-positive rate with non-inserted keys.
	fp := 0
	tests := 100000
	for i := range tests {
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
	for i := range 100 {
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

func TestLookupMetaGet(t *testing.T) {
	w := NewWriter()
	w.SetIndexInterval(4)
	for i := range 32 {
		w.Add([]byte("key-"+zeroPad(i, 3)), []byte("val-"+strconv.Itoa(i)))
	}

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}
	data := buf.Bytes()

	meta, err := LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("LoadLookupMeta: %v", err)
	}
	if string(meta.MinKey) != "key-000" {
		t.Fatalf("min key: got %q", meta.MinKey)
	}
	if string(meta.MaxKey) != "key-031" {
		t.Fatalf("max key: got %q", meta.MaxKey)
	}
	if len(meta.Index) == 0 {
		t.Fatal("expected sparse index entries")
	}
	if meta.Bloom == nil {
		t.Fatal("expected bloom filter")
	}

	val, found, err := meta.Get(bytes.NewReader(data), []byte("key-017"))
	if err != nil {
		t.Fatalf("LookupMeta.Get: %v", err)
	}
	if !found || string(val) != "val-17" {
		t.Fatalf("LookupMeta.Get: found=%v val=%q want val-17", found, val)
	}

	_, found, err = meta.Get(bytes.NewReader(data), []byte("key-999"))
	if err != nil {
		t.Fatalf("LookupMeta.Get(missing): %v", err)
	}
	if found {
		t.Fatal("LookupMeta.Get(missing): should not be found")
	}
}

func TestLookupMetaHas(t *testing.T) {
	w := NewWriter()
	w.SetIndexInterval(4)
	for i := range 32 {
		w.Add([]byte("key-"+zeroPad(i, 3)), []byte("val-"+strconv.Itoa(i)))
	}
	w.AddTombstone([]byte("key-040"))

	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		t.Fatalf("Build: %v", err)
	}
	data := buf.Bytes()

	meta, err := LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("LoadLookupMeta: %v", err)
	}

	found, err := meta.Has(bytes.NewReader(data), []byte("key-017"))
	if err != nil {
		t.Fatalf("LookupMeta.Has(existing): %v", err)
	}
	if !found {
		t.Fatal("LookupMeta.Has(existing): not found")
	}

	found, err = meta.Has(bytes.NewReader(data), []byte("key-040"))
	if err != nil {
		t.Fatalf("LookupMeta.Has(tombstone): %v", err)
	}
	if found {
		t.Fatal("LookupMeta.Has(tombstone): should not be found")
	}

	found, err = meta.Has(bytes.NewReader(data), []byte("key-999"))
	if err != nil {
		t.Fatalf("LookupMeta.Has(missing): %v", err)
	}
	if found {
		t.Fatal("LookupMeta.Has(missing): should not be found")
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

type countingReaderAt struct {
	data  []byte
	reads int
}

func (r *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	return bytes.NewReader(r.data).ReadAt(p, off)
}

type recordingReaderAt struct {
	data    []byte
	reads   int
	maxRead int
}

func (r *recordingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	r.maxRead = max(r.maxRead, len(p))
	return bytes.NewReader(r.data).ReadAt(p, off)
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
