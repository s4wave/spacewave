//go:build !tinygo

package block_store_s3

// VersionCopyResult counts what a version copy wrote, or would write in a dry
// run.
type VersionCopyResult struct {
	// Keys is the keys copied.
	Keys uint64
	// Versions is the versions copied.
	Versions uint64
	// Markers is the delete markers recreated.
	Markers uint64
	// Bytes is the copied versions' bytes.
	Bytes uint64
	// Largest is the largest version listed, in bytes. A single-request
	// copy of a version over 5 GiB fails.
	Largest uint64
	// Skipped is the keys whose destination already held the same history.
	Skipped uint64
	// Resumed is the keys whose destination held the start of the history,
	// left by a copy stopped partway, and were finished.
	Resumed uint64
	// Conflicts is the keys whose destination held another history, left
	// alone.
	Conflicts uint64
	// ConflictKeys lists the first conflicting source keys.
	ConflictKeys []string
}
