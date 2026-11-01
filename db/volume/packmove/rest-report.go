package packmove

// RestReport counts the keys outside the volume's block range, which Switch
// copies whole, so it sizes the new file apart from the blocks it keeps.
type RestReport struct {
	// Keys is the number of keys counted.
	Keys uint64
	// Bytes is the size of those keys and their values.
	Bytes uint64
	// Done reports that the count reached the end of the key space.
	Done bool
}
