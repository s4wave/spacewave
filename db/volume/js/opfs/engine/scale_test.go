//go:build !js

package engine

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestDurableIndexScale proves bounded reopen, lookup, scan cache, and deletion
// behavior beyond the catalogue's first branching threshold.
func TestDurableIndexScale(t *testing.T) {
	ctx := t.Context()
	d := newDiskBackend(t)
	e, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()

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
		t.Helper()
		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
		e, err = Open(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		resetReads()
		got, found, _, err := e.Get(ctx, key(count/2))
		if err != nil || !found || !bytes.Equal(got, value) {
			t.Fatalf("get present key at %d: found=%t error=%v", count, found, err)
		}
		if _, found, _, err := e.Get(ctx, key(count)); err != nil || found {
			t.Fatalf("get absent key at %d: found=%t error=%v", count, found, err)
		}
		reads, payloadReads := readCounts()
		t.Logf("%d keys: two reopened lookups read %d files (%d payload files)", count, reads, payloadReads)
		if payloadReads != 0 {
			t.Fatalf("%d keys: lookups visited %d payload files", count, payloadReads)
		}
		if reads > 16 {
			t.Fatalf("%d keys: reopened lookups visited %d files", count, reads)
		}
	}

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
	e.mtx.Lock()
	cacheBytes, cacheFiles := e.cacheBytes, len(e.cache)
	e.mtx.Unlock()
	t.Logf("full scan cache: %d bytes in %d files", cacheBytes, cacheFiles)
	if cacheBytes > cacheByteLimit || cacheFiles > cacheFileLimit {
		t.Fatalf("cache exceeded limits: %d bytes in %d files", cacheBytes, cacheFiles)
	}
	tx.Discard()

	for start := 0; start < 32768; start += 512 {
		records := make([]*Record, 0, 512)
		for index := start; index < start+512; index++ {
			records = append(records, &Record{Key: key(index), Deleted: true})
		}
		if err := e.Apply(ctx, records); err != nil {
			t.Fatalf("delete through key %d: %v", start+511, err)
		}
	}
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
	reads, payloadReads := readCounts()
	t.Logf("empty reopened lookup and scan read %d files (%d payload files)", reads, payloadReads)
	if payloadReads != 0 {
		t.Fatalf("empty reopen visited %d payload files", payloadReads)
	}
	if reads > 8 {
		t.Fatalf("empty reopened lookup and scan visited %d files", reads)
	}
}
