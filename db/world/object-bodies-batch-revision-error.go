package world

import "strconv"

// MaxObjectBodiesBatchRevisionRetries bounds the restarts of
// GetObjectBodiesBatch when a write lands between its pages.
const MaxObjectBodiesBatchRevisionRetries = 3

// ObjectBodiesBatchRevisionError reports that paginated body reads crossed
// World sequence numbers too many times to form one consistent result.
type ObjectBodiesBatchRevisionError struct {
	// Expected is the sequence number recorded from the first page.
	Expected uint64
	// Got is the sequence number observed on the mismatching page.
	Got uint64
	// Retries is the number of pagination restarts attempted.
	Retries int
}

// Error returns the inconsistent World sequence details.
func (e *ObjectBodiesBatchRevisionError) Error() string {
	return "object bodies batch World sequence changed from " +
		strconv.FormatUint(e.Expected, 10) + " to " +
		strconv.FormatUint(e.Got, 10) + " after " +
		strconv.Itoa(e.Retries) + " retries"
}
