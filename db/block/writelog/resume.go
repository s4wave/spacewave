package writelog

import (
	"bytes"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

// markerName is the resume marker's file in a log directory. Its first line is
// the position of a closed epoch's close line, as Position.String writes it,
// and its second, when the log had a witness at Close, the witness in hex. It
// exists only while the epoch's segments are durable through that line and no
// write has escaped the log since. Claim takes it before the store's first
// write, so it never outlives the process that claims the directory.
const markerName = "resume"

// Witness returns a fingerprint of the store's files as they stand, such as
// each file's size, modification time and inode, so a write no log observed
// between one process's Close and the next one's Claim changes it. An error
// means the files cannot be vouched for, and no epoch resumes.
type Witness func() ([]byte, error)

// marker is a parsed resume marker.
type marker struct {
	// pos is the closed epoch's close line.
	pos Position
	// witness is the store's fingerprint at Close, empty when none was taken.
	witness []byte
}

// parseMarker parses a marker's contents. A marker of one line, as written
// before witnesses, parses with no witness.
func parseMarker(data []byte) (marker, error) {
	// Split the position line from the witness line.
	line, rest, _ := bytes.Cut(bytes.TrimSpace(data), []byte{'\n'})
	pos, err := ParsePosition(string(line))
	if err != nil {
		return marker{}, err
	}
	witness, err := hex.DecodeString(string(bytes.TrimSpace(rest)))
	if err != nil {
		return marker{}, err
	}
	return marker{pos: pos, witness: witness}, nil
}

// markerPath returns the resume marker's path in dir.
func markerPath(dir string) string {
	return filepath.Join(dir, markerName)
}

// writeMarker durably writes the marker naming pos and witness in dir,
// replacing any.
func writeMarker(dir string, pos Position, witness []byte) error {
	// Format the position line and any witness line.
	data := pos.String() + "\n"
	if len(witness) != 0 {
		data += hex.EncodeToString(witness) + "\n"
	}

	// Write a temporary file and sync it.
	tmp := markerPath(dir) + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = f.WriteString(data)
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

// stuckMarker is logged when a marker can be neither removed nor poisoned.
// This process never resumes from it, but the marker stays valid on disk, and
// a later start that can remove it would resume past this process's writes.
// No file in the directory can record the refusal durably, since writing one
// needs the access the removal lacked, so the operator removes the marker.
const stuckMarker = "write log: the resume marker could be neither removed nor poisoned; remove it before the next start, which would otherwise resume past this process's writes"

// takeMarker retires the marker of dir and returns what it named, whether the
// epoch there may resume at it, and whether the retirement is durable, as
// retireMarker reports. The epoch may resume when the marker parsed, its
// removal is durable, and the epoch's last segment ends at its close line.
func takeMarker(dir string) (marker, bool, bool) {
	// Retire the marker; only a durable removal may resume.
	data, durable := retireMarker(dir)
	if data == nil || !durable {
		return marker{}, false, durable
	}

	// Parse the marker and check the epoch still ends there.
	m, err := parseMarker(data)
	if err != nil {
		return marker{}, false, true
	}
	return m, closedAt(dir, m.pos), true
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

	// Remove it and sync the directory; one another retirement removed first
	// still needs the sync.
	if err == nil {
		err = os.Remove(markerPath(dir))
		if os.IsNotExist(err) {
			err = nil
		}
	}
	if err == nil {
		err = syncDir(dir)
	}
	if err == nil {
		return data, true
	}

	// Poison the epoch the marker names, when it can still be read.
	m, perr := parseMarker(data)
	if data == nil || perr != nil {
		return data, false
	}
	return data, poisonEpoch(dir, m.pos) == nil
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
