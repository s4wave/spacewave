package packmove

import (
	"bytes"
	"io"
	"os"

	"github.com/pkg/errors"
)

// keyState is what the pack log recorded for a block key.
type keyState int

const (
	// keyUnrecorded is a key the log committed no entry for.
	keyUnrecorded keyState = iota
	// keyPacked is a block committed in a pack.
	keyPacked
	// keyBad is a block Pack found with data that fails its hash and left out
	// of every pack.
	keyBad
)

// packedCursor reads the committed entries of a sequence of pack logs in key
// order, to answer what the logs recorded for a key. Each log covers a key
// range after the one before it, as the shards of one Pack run do. Lookups must come in key order; the
// cursor moves forward only, so a pass over the log costs one sequential read.
type packedCursor struct {
	// paths are the logs not yet opened.
	paths []string
	// file is the log being read.
	file *os.File
	// frames decodes the log.
	frames *frameReader
	// pending is the batch opened and not yet committed.
	pending *frame
	// batch holds the entries of the committed batch being read.
	batch []logEntry
	// head is the next committed entry not before the last lookup, or nil
	// before the first entry and after the last.
	head *logEntry
	// done reports that the log has no more keys.
	done bool
}

// openPackedCursor opens the logs at paths for reading, in key order.
func openPackedCursor(paths []string) (*packedCursor, error) {
	if len(paths) == 0 {
		return nil, errors.New("no pack log")
	}
	c := &packedCursor{paths: paths}
	if err := c.openNext(); err != nil {
		return nil, err
	}
	return c, nil
}

// openNext closes the log being read and opens the next one, dropping a batch
// the closed log opened and never committed.
func (c *packedCursor) openNext() error {
	if c.file != nil {
		if err := c.file.Close(); err != nil {
			return err
		}
	}
	f, err := os.Open(c.paths[0]) // #nosec G304 -- the operator chooses the state directory.
	if err != nil {
		return err
	}
	c.file, c.frames, c.pending, c.paths = f, newFrameReader(f), nil, c.paths[1:]
	return nil
}

// state returns what the log committed for key. Each key must sort after the
// one before it.
func (c *packedCursor) state(key []byte) (keyState, error) {
	for {
		if c.head == nil {
			if c.done {
				return keyUnrecorded, nil
			}
			if err := c.advance(); err != nil {
				return keyUnrecorded, err
			}
			continue
		}
		cmp := bytes.Compare(c.head.key, key)
		switch {
		case cmp < 0:
			c.head = nil
		case cmp > 0:
			return keyUnrecorded, nil
		case c.head.bad:
			return keyBad, nil
		default:
			return keyPacked, nil
		}
	}
}

// advance loads the next committed entry into head, or sets done at the end
// of the last log. A batch without a commit frame, or a torn frame, ends its
// log.
func (c *packedCursor) advance() error {
	for len(c.batch) == 0 {
		next, err := c.frames.next()
		if (errors.Is(err, io.EOF) || errors.Is(err, errTornFrame)) && len(c.paths) != 0 {
			if err := c.openNext(); err != nil {
				return err
			}
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(err, errTornFrame) {
			c.done = true
			return nil
		}
		if err != nil {
			return err
		}
		if next.kind == framePending {
			c.pending = &next
			continue
		}
		if c.pending == nil {
			return errors.New("pack log commits a batch it never opened")
		}
		c.batch, c.pending = c.pending.entries, nil
	}
	c.head, c.batch = &c.batch[0], c.batch[1:]
	return nil
}

// close closes the log being read.
func (c *packedCursor) close() error {
	return c.file.Close()
}
