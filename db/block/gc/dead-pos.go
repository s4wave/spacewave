package block_gc

import "encoding/binary"

// DeadPos is the place of a block at zero holders on the free queue and
// among the parked keys: the order number its down cascade took, zero until
// the cascade runs, then its ripe sequence, which the cursor must pass
// before the collector frees it. Both sort by the order first, so a parent
// is freed before every child its release brought to zero, whatever their
// ripe sequences: a stored parent can be found and held again, and its
// re-hold must find its children stored.
type DeadPos struct {
	// order is the order number of the block's down cascade.
	order uint64
	// ripe is the block's ripe sequence.
	ripe uint64
}

// queuePos returns the position on the free queue of the block at key at
// pos: its queue key without the prefix and kind, the order, then the ripe
// sequence, then the block key.
func queuePos(pos DeadPos, key []byte) []byte {
	b := binary.BigEndian.AppendUint64(make([]byte, 0, 16+len(key)), pos.order)
	return append(binary.BigEndian.AppendUint64(b, pos.ripe), key...)
}
