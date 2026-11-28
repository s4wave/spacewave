//go:build darwin || linux || windows

package s4dbv1

import (
	"bytes"
	"io"

	"github.com/pkg/errors"
	"github.com/tidwall/btree"
)

// oentry is one key's newest change in the log after the checkpoint.
type oentry struct {
	// key is the key.
	key []byte
	// val is the value or a tombstone.
	val value
}

// lessEntry orders entries by key.
func lessEntry(a, b oentry) bool {
	return bytes.Compare(a.key, b.key) < 0
}

// replayLog reads the log after sb and returns its changes keyed by key, the
// newest winning. It applies the previous engine's recovery rule: a record
// past the highest durable mark whose extents do not read is the torn tail,
// and it and everything after it are left out. An extent reads as torn only
// when its bytes fail their checksum or lie past the end of the file. Any
// other read error fails the replay, so a failing drive never shortens the
// log a conversion copies.
func replayLog(r io.ReaderAt, sb superblock) (*btree.BTreeG[oentry], error) {
	// Read every valid record and the highest mark among them.
	var recs []*record
	var durable uint64
	lr := newLogReader(r, sb)
	for {
		rec, ok := lr.next()
		if !ok {
			break
		}
		recs = append(recs, rec)
		durable = max(durable, rec.durable)
	}
	if lr.err != nil {
		return nil, lr.err
	}

	// Apply the commits up to the torn tail.
	ov := btree.NewBTreeGOptions(lessEntry, btree.Options{NoLocks: true})
	for _, rec := range recs {
		if rec.kind != kindCommit {
			continue
		}
		if rec.seq > durable {
			ok, err := extentsRead(r, rec)
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
		}
		for _, o := range rec.ops {
			ov.Set(oentry(o))
		}
	}
	return ov, nil
}

// extentsRead reports whether every extent a record references reads whole,
// returning an error for a read that fails for another reason than a torn
// extent.
func extentsRead(r io.ReaderAt, rec *record) (bool, error) {
	for _, o := range rec.ops {
		if !o.val.isRef {
			continue
		}
		_, err := o.val.read(r)
		switch {
		case err == nil:
		case errors.Is(err, ErrCorrupt), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
			return false, nil
		default:
			return false, errors.Wrapf(err, "read extent at %d", o.val.ref.off)
		}
	}
	return true, nil
}
