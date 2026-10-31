package s4db

// SlotPin is the oldest state a reader slot's snapshots read.
type SlotPin struct {
	// Slot is the slot index.
	Slot int
	// Seq is the oldest commit the snapshots read.
	Seq uint64
	// Ckpt is the oldest checkpoint the snapshots read.
	Ckpt uint64
	// Live reports whether an open handle holds the slot. A pin in a slot
	// no handle holds keeps nothing allocated.
	Live bool
	// Own reports whether the slot is this handle's.
	Own bool
}
