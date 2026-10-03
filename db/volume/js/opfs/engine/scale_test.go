//go:build !js

package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"testing"
)

// TestDurableIndexScale proves bounded reopen, lookup, scan cache, and deletion
// behavior beyond the catalogue's first branching threshold.
func TestDurableIndexScale(t *testing.T) {
	// Open a durable engine for index scale checks.
	ctx := t.Context()
	d := newDiskBackend(t)
	e, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()

	// Define ordered keys and synchronized backend read counters.
	key := func(index int) []byte {
		encoded := make([]byte, 8)
		binary.BigEndian.PutUint64(encoded, uint64(index))
		return encoded
	}
	value := bytes.Repeat([]byte("v"), 1024)
	resetReads := func() {
		d.mtx.Lock()
		d.reads = 0
		d.payloadReads = 0
		d.mtx.Unlock()
	}
	readCounts := func() (int, int) {
		d.mtx.Lock()
		defer d.mtx.Unlock()
		return d.reads, d.payloadReads
	}
	reopenAndProbe := func(count int) {
		// Reopen the engine before measuring lookup work.
		t.Helper()
		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
		e, err = Open(ctx, d)
		if err != nil {
			t.Fatal(err)
		}

		// Verify present and absent key lookups after reopening.
		resetReads()
		got, found, _, err := e.Get(ctx, key(count/2))
		if err != nil || !found || !bytes.Equal(got, value) {
			t.Fatalf("get present key at %d: found=%t error=%v", count, found, err)
		}
		if _, found, _, err := e.Get(ctx, key(count)); err != nil || found {
			t.Fatalf("get absent key at %d: found=%t error=%v", count, found, err)
		}

		// Require reopened lookups to use bounded index reads without payload visits.
		reads, payloadReads := readCounts()
		t.Logf("%d keys: two reopened lookups read %d files (%d payload files)", count, reads, payloadReads)
		if payloadReads != 0 {
			t.Fatalf("%d keys: lookups visited %d payload files", count, payloadReads)
		}
		if reads > 16 {
			t.Fatalf("%d keys: reopened lookups visited %d files", count, reads)
		}
	}

	// Populate and probe the index across its branching thresholds.
	previous := 0
	for _, target := range []int{1024, 32768} {
		for start := previous; start < target; start += 512 {
			records := make([]*Record, 0, 512)
			for index := start; index < start+512; index++ {
				records = append(records, &Record{Key: key(index), Value: value})
			}
			if err := e.Apply(ctx, records); err != nil {
				t.Fatalf("populate through key %d: %v", start+511, err)
			}
		}
		reopenAndProbe(target)
		previous = target
	}

	// Scan all ordered records through one retained transaction.
	tx, err := e.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	var scanned int
	if err := tx.ScanPrefix(ctx, nil, func(gotKey, gotValue []byte) error {
		if !bytes.Equal(gotKey, key(scanned)) || !bytes.Equal(gotValue, value) {
			t.Fatalf("scan record %d did not match", scanned)
		}
		scanned++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if scanned != 32768 {
		t.Fatalf("scan visited %d records", scanned)
	}

	// Require a full scan to respect cache byte and file limits.
	e.mtx.Lock()
	cacheBytes, cacheFiles := e.cacheBytes, len(e.cache)
	e.mtx.Unlock()
	t.Logf("full scan cache: %d bytes in %d files", cacheBytes, cacheFiles)
	if cacheBytes > cacheByteLimit || cacheFiles > cacheFileLimit {
		t.Fatalf("cache exceeded limits: %d bytes in %d files", cacheBytes, cacheFiles)
	}
	tx.Discard()

	// Delete every populated key in bounded publication batches.
	for start := 0; start < 32768; start += 512 {
		records := make([]*Record, 0, 512)
		for index := start; index < start+512; index++ {
			records = append(records, &Record{Key: key(index), Deleted: true})
		}
		if err := e.Apply(ctx, records); err != nil {
			t.Fatalf("delete through key %d: %v", start+511, err)
		}
	}

	// Reopen the emptied index and verify the deleted key is absent.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	resetReads()
	if _, found, _, err := e.Get(ctx, key(0)); err != nil || found {
		t.Fatalf("get after deletion: found=%t error=%v", found, err)
	}

	// Require the reopened index scan to yield no records.
	tx, err = e.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	scanned = 0
	if err := tx.ScanPrefix(ctx, nil, func(_, _ []byte) error {
		scanned++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if scanned != 0 {
		t.Fatalf("scan after deletion visited %d records", scanned)
	}

	// Require the empty index lookup and scan to use bounded reads.
	reads, payloadReads := readCounts()
	t.Logf("empty reopened lookup and scan read %d files (%d payload files)", reads, payloadReads)
	if payloadReads != 0 {
		t.Fatalf("empty reopen visited %d payload files", payloadReads)
	}
	if reads > 8 {
		t.Fatalf("empty reopened lookup and scan visited %d files", reads)
	}
}

// TestScatteredKeyWriteAmplification proves batches spread across every
// partition, like a queue keyed by block hash, rewrite each record a bounded
// number of times while filling and draining.
func TestScatteredKeyWriteAmplification(t *testing.T) {
	// Open a durable engine for scattered-key write measurements.
	ctx := t.Context()
	d := newDiskBackend(t)
	e, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()

	// Populate and delete hash-distributed keys in sorted batches.
	const count, batch = 32768, 512
	key := func(index int) []byte {
		sum := sha256.Sum256(binary.BigEndian.AppendUint64(nil, uint64(index)))
		return sum[:]
	}
	value := bytes.Repeat([]byte("v"), 48)
	apply := func(deleted bool) {
		t.Helper()
		for start := 0; start < count; start += batch {
			records := make([]*Record, 0, batch)
			for index := start; index < start+batch; index++ {
				record := &Record{Key: key(index), Deleted: deleted}
				if !deleted {
					record.Value = value
				}
				records = append(records, record)
			}
			slices.SortFunc(records, func(a, b *Record) int { return bytes.Compare(a.Key, b.Key) })
			if err := e.Apply(ctx, records); err != nil {
				t.Fatal(err)
			}
		}
	}
	apply(false)
	apply(true)

	// Compare physical run writes with the logical record volume.
	d.mtx.Lock()
	written := d.kindBytes["run"]
	d.mtx.Unlock()
	logical := int64(count * (len(key(0)) + len(value)))
	t.Logf("%d scattered keys wrote %d run bytes for %d logical bytes", count, written, logical)
	if written > 12*logical {
		t.Fatalf("scattered keys wrote %d run bytes, over 12x the %d logical bytes", written, logical)
	}
}
