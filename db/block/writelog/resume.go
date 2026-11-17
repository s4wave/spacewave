package writelog

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
)

// markerName is the resume marker's file in a log directory. It holds the
// position of a closed epoch's close line, as Position.String writes it, and
// exists only while the epoch's segments are durable through that line and no
// write has escaped the log since.
const markerName = "resume"

// markerPath returns the resume marker's path in dir.
func markerPath(dir string) string {
	return filepath.Join(dir, markerName)
}

// writeMarker durably writes the marker naming pos in dir, replacing any.
func writeMarker(dir string, pos Position) error {
	// Write a temporary file and sync it.
	tmp := markerPath(dir) + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = f.WriteString(pos.String() + "\n")
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}

	// Rename it into place.
	return os.Rename(tmp, markerPath(dir))
}

// takeMarker reads and removes the marker of dir, durably, returning the
// position it names and whether it named one.
func takeMarker(dir string) (Position, bool, error) {
	// Read the marker; none names nothing.
	data, err := os.ReadFile(markerPath(dir))
	if os.IsNotExist(err) {
		return Position{}, false, nil
	}
	if err != nil {
		return Position{}, false, err
	}

	// Remove it before any line follows, then parse it. A removal the
	// directory cannot make durable resumes nothing.
	if err := os.Remove(markerPath(dir)); err != nil {
		return Position{}, false, err
	}
	if err := syncDir(dir); err != nil {
		return Position{}, false, nil
	}
	pos, err := ParsePosition(string(bytes.TrimSpace(data)))
	return pos, err == nil, nil
}

// closedAt reports whether the last segment of pos's epoch in dir ends with
// the epoch's close line at pos.
func closedAt(dir string, pos Position) bool {
	// Find the epoch's last segment.
	segs, err := listSegments(dir)
	if err != nil {
		return false
	}
	segs = epochSegments(segs, pos.Epoch)
	if len(segs) == 0 {
		return false
	}
	last := segs[len(segs)-1]

	// Read its tail and compare it with the close line.
	want := append([]byte{'\n'}, appendMarker(nil, pos.Seq, kindClose, pos.Epoch)...)
	f, err := os.Open(last.path(dir))
	if err != nil {
		return false
	}
	defer f.Close()
	if last.size < int64(len(want)) {
		return false
	}
	tail := make([]byte, len(want))
	if _, err := f.ReadAt(tail, last.size-int64(len(want))); err != nil && err != io.EOF {
		return false
	}
	return bytes.Equal(tail, want)
}

// syncFile syncs the file at path.
func syncFile(path string) error {
	// Open the file, sync it, and close it.
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = f.Sync()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// syncDir syncs the directory dir, so its entries survive a host crash.
func syncDir(dir string) error {
	return syncFile(dir)
}
