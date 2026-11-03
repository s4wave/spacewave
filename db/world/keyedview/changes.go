package keyedview

// Changes is what a reader lacks to go from its version to a View's.
type Changes struct {
	// Full reports that the reader's version is unknown: Changed lists every
	// key, and the reader drops what it held first.
	Full bool
	// Changed lists the keys added or changed, in key order.
	Changed []string
	// Removed lists the keys the reader holds that are gone, in key order.
	Removed []string
}
