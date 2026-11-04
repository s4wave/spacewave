package block

import "testing"

// TestDecodedBlockReadSlotRetirement proves held ancestor links cannot retain
// evicted payloads or reattach metadata to a superseded pool entry.
func TestDecodedBlockReadSlotRetirement(t *testing.T) {
	// Give every retirement path a fresh admitted entry with attached metadata.
	for _, mode := range []string{"remove", "replace", "evict", "clear"} {
		t.Run(mode, func(t *testing.T) {
			pool := newDecodedBlockPoolShards(1024)
			slot := &decodedBlockReadSlot{}
			data := &decodedBlockReadData{links: map[uint32]*decodedBlockReadLink{1: {}}}
			if !pool.put("held", decodedBlockCacheEntry{read: slot}, 512) || !pool.attachReadData("held", slot, data, 128) {
				t.Fatal("failed to admit traversal metadata")
			}

			// Retire the entry while an ancestor still holds its slot.
			switch mode {
			case "remove":
				pool.del("held")
			case "replace":
				pool.put("held", decodedBlockCacheEntry{read: &decodedBlockReadSlot{}}, 512)
			case "evict":
				pool.put("replacement", decodedBlockCacheEntry{}, 1024)
			case "clear":
				pool.clear()
			}
			if slot.data.Load() != nil {
				t.Fatal("ancestor slot retained retired payload")
			}
			if pool.attachReadData("held", slot, data, 128) {
				t.Fatal("stale fill resurrected retired slot")
			}
			if retained := pool.stats().retained; retained > 1024 {
				t.Fatalf("retained %d exceeds budget", retained)
			}
		})
	}
}
