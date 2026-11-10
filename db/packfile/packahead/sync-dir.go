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
