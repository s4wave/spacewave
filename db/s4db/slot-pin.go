package s4db

// SlotPin is the state a reader slot's snapshots read.
type SlotPin struct {
	// Slot is the slot index.
	Slot int
	// Seq is the oldest commit the snapshots read.
	Seq uint64
	// Version is the oldest manifest version the snapshots read.
	Version uint64
	// Own reports whether the slot is this handle's.
	Own bool
}
