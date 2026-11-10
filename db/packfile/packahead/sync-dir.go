package packahead

import "os"

// dirSync is syncDir, replaced by a test to fail a directory sync.
var dirSync = syncDir

// syncDir makes the directory's entries, its creates, renames and removes,
// durable on the drive.
func syncDir(dir string) error {
	f, err := os.Open(dir) // #nosec G703 -- the Log's own directory.
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// batchWriteAt writes a batch to a pack file, replaced by a test to fail it.
var batchWriteAt = (*os.File).WriteAt

// indexWriteAt writes a pack's index, replaced by a test to fail it.
var indexWriteAt = (*os.File).WriteAt

// indexSync syncs a pack's index, replaced by a test to fail it.
var indexSync = func(opts Options, f *os.File) error { return opts.SyncData(f) }

// renameFile renames a pack to its finalized name, replaced by a test to
// fail it.
var renameFile = os.Rename
