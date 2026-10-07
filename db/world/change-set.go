package world

// ChangeSet describes the World changes between two sequence numbers.
type ChangeSet struct {
	// Unknown means the changes could not be determined.
	// Every ReadSet is touched by an unknown ChangeSet.
	Unknown bool
	// Keys are the object keys that were set, deleted, renamed, or revised.
	// A rename lists both the old and the new key.
	Keys []string
	// Quads are the graph quads that were set or deleted.
	Quads []GraphQuad
}

// NewUnknownChangeSet returns a ChangeSet whose changes are unknown.
func NewUnknownChangeSet() *ChangeSet {
	return &ChangeSet{Unknown: true}
}
