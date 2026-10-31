package s4db

import "context"

// Sizes is the file's length, its live values, and the disk a collection
// needs before it frees any.
type Sizes struct {
	// FileBytes is the file length.
	FileBytes int64
	// ValueBytes is the total length of the live values.
	ValueBytes int64
	// CollectBytes bounds the disk that deleting keys takes before the space
	// they free returns to the disk.
	CollectBytes int64
}

// Sizes reports the file's sizes as of the latest commit.
func (db *DB) Sizes(ctx context.Context) (Sizes, error) {
	return Sizes{}, ErrUnimplemented
}
