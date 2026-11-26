package packahead

import "context"

// DrainOptions configures a Drain.
type DrainOptions struct {
	// KeepBytes is the size of finalized packs kept local after upload, as a
	// read cache. Beyond it the oldest uploaded packs are evicted. Zero keeps
	// every pack, uploading without evicting.
	KeepBytes int64
	// Admit paces the uploads: it is called before each with the bytes of the
	// previous one, and may wait. Nil uploads at once.
	Admit func(ctx context.Context, lastBytes int64) error
}
