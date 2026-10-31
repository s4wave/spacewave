package s4db

// Flow is the length of the values commits stored and freed under one key
// family since the database opened.
type Flow struct {
	// Written is the length of the values commits stored.
	Written int64
	// Freed is the length of the stored values commits replaced or deleted.
	Freed int64
}

// Flows returns the flow of each of Options.Families in order, then of the
// keys under none.
func (db *DB) Flows() []Flow {
	return nil
}
