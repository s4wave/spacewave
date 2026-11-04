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

			// Count the entry and its metadata while attached.
			if st := pool.stats(); st.entries != 1 || st.readCost != 128 || st.readSlots != 1 {
				t.Fatalf("attached counters %+v", st)
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
			st := pool.stats()
			if st.retained > 1024 {
				t.Fatalf("retained %d exceeds budget", st.retained)
			}

			// Uncount the retired metadata and keep the entry count exact.
			wantEntries := int64(1)
			if mode == "remove" || mode == "clear" {
				wantEntries = 0
			}
			if st.readCost != 0 || st.readSlots != 0 || st.entries != wantEntries {
				t.Fatalf("retired counters %+v want %d entries", st, wantEntries)
			}
		})
	}
}
