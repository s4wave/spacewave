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
// write has escaped the log since. Claim takes it before the store's first
// write, so it never outlives the process that claims the directory.
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

	// Rename it into place, and make the rename durable.
	if err := os.Rename(tmp, markerPath(dir)); err != nil {
		return err
	}
	return syncDir(dir)
}

// takeMarker retires the marker of dir and returns the position it named,
// and whether the epoch there may resume at it: the marker parsed, its
// removal is durable, and the epoch's last segment ends at its close line.
func takeMarker(dir string) (Position, bool) {
	// Retire the marker; only a durable removal may resume.
	data, durable := retireMarker(dir)
	if data == nil || !durable {
		return Position{}, false
	}

	// Parse the position and check the epoch still ends there.
	pos, err := ParsePosition(string(bytes.TrimSpace(data)))
	if err != nil {
		return Position{}, false
	}
	return pos, closedAt(dir, pos)
}

// retireMarker removes the marker of dir, durably, returning its contents,
// nil when there was none, and whether its removal is durable. When the
// removal or the directory sync fails, a crash could bring the marker back,
// so it poisons the named epoch's last segment instead: a byte after the
// close line fails closedAt, and every reader still stops at the close line.
func retireMarker(dir string) ([]byte, bool) {
	// Read the marker; none needs nothing.
	data, err := os.ReadFile(markerPath(dir))
	if os.IsNotExist(err) {
		return nil, true
	}

	// Remove it and sync the directory.
	if err == nil {
		err = os.Remove(markerPath(dir))
	}
	if err == nil {
		err = syncDir(dir)
	}
	if err == nil {
		return data, true
	}

	// Poison the epoch the marker names, when it can still be read.
	pos, perr := ParsePosition(string(bytes.TrimSpace(data)))
	if data == nil || perr != nil {
		return data, false
	}
	return data, poisonEpoch(dir, pos) == nil
}

// poisonEpoch durably appends a byte to the last segment of pos's epoch in
// dir, so closedAt fails for it.
func poisonEpoch(dir string, pos Position) error {
	// Find the epoch's last segment.
	segs, err := listSegments(dir)
	if err != nil {
		return err
	}
	segs = epochSegments(segs, pos.Epoch)
	if len(segs) == 0 {
		return nil
	}

	// Append the byte and sync it.
	f, err := os.OpenFile(segs[len(segs)-1].path(dir), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte{'\n'})
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
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
