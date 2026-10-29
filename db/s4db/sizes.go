package s4db

import "context"

// collectSlackBytes is the disk a collection writes beside its log records
// and its checkpoint: a log chunk, a punch batch, and the edits of the
// collector's own records.
const collectSlackBytes = 64 << 20

// Sizes is the file's length, its live values, and the disk a collection
// needs before it frees any, as of the latest commit.
type Sizes struct {
	// FileBytes is the file length.
	FileBytes int64
	// ValueBytes is the total length of the live values stored outside the
	// index.
	ValueBytes int64
	// CollectBytes bounds the disk that deleting keys takes before the space
	// they free returns to the disk: the log a commit may append before it
	// waits for a checkpoint, a checkpoint that rewrites the whole index, and
	// a slack. Freed pages are punched, so a write that reuses them takes disk
	// again, even inside the file's length.
	CollectBytes int64
}

// Sizes reports the file's sizes as of the latest commit. It takes the writer
// lock for a few reads, where Usage visits every free run under it, so a
// caller may measure often without holding up writes.
func (db *DB) Sizes(ctx context.Context) (Sizes, error) {
	// Read the space's counts under the writer lock, which keeps the space.
	if err := db.w.lock(ctx); err != nil {
		return Sizes{}, err
	}
	s := Sizes{FileBytes: pageOff(db.w.sp.end()), ValueBytes: db.w.sp.liveBytes}
	treePages := db.cur.Load().sb.treePages
	db.w.unlock(nil)

	// A commit waits for the checkpoint once the log passes four times the
	// largest checkpoint limit.
	s.CollectBytes = pageOff(treePages) + 4*db.opts.CheckpointMax + collectSlackBytes
	return s, nil
}
