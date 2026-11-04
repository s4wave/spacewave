package s4db

import (
	"encoding/binary"
	"runtime"
	"testing"
)

// TestOverlayRetention checks that cold keys do not keep replaced hot entries alive.
func TestOverlayRetention(t *testing.T) {
	// Measure the heap before applying mixed hot and cold commits.
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	st := &state{overlay: newOverlay(), filter: newFilter(1 << 20)}

	// Keep one distinct cold key from each batch and replace all hot keys.
	for batch := range 256 {
		r := &record{kind: kindCommit, seq: uint64(batch + 1)}
		for i := range 1024 {
			key := make([]byte, 16)
			binary.BigEndian.PutUint64(key[8:], uint64(i))
			if i == 0 {
				binary.BigEndian.PutUint64(key, uint64(batch+1))
			}
			r.ops = append(r.ops, op{key: key, val: value{inline: make([]byte, 128)}})
		}
		r.own()
		st.apply(r)
	}

	// Leave ample room for runtime noise while rejecting whole-batch retention.
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(st)
	retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if retained > 16<<20 {
		t.Fatalf("cold keys retain %d bytes for 1279 live entries", retained)
	}
}
