package s4db

// FreeRunClass counts the free runs of one length class.
type FreeRunClass struct {
	// MinBytes is the shortest run of the class. The longest is just under
	// twice it.
	MinBytes int64
	// Runs counts the runs.
	Runs int64
	// Bytes is the total length of the runs.
	Bytes int64
}
