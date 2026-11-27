// Package keyedview tells a reader what changed in a keyed view since the
// version it holds.
//
// A keyed view is a result made of records with stable keys, such as
// playlists by ID. Each result gets a version named by its content: the
// digest of every record's key and encoding. A reader that supplies the version
// it holds learns which keys changed and which went away, so it receives only
// those records. A reader whose version is unknown, because the reader never
// read the view or the History forgot the version, receives every record once.
// A view also has a scope, such as the map its records belong to. A reader
// whose version belongs to another scope receives every record once too, since
// nothing it holds carries over.
package keyedview

import (
	"encoding/binary"
	"slices"
	"strconv"

	"github.com/cespare/xxhash/v2"
)

// Record is a record of a keyed view.
type Record interface {
	// MarshalVT encodes the record. Equal records encode equally.
	MarshalVT() ([]byte, error)
}

// View is one result of a keyed view: each record's digest by key, and the
// version that identifies them all. A View is immutable once built.
type View struct {
	// Version identifies the result's scope and content.
	// Equal results share it.
	Version string
	// scope identifies what the records belong to.
	// Views of different scopes share no records.
	scope string
	// digests holds each record's digest by key.
	digests map[string]uint64
}

// Build digests records of scope into a View.
func Build[T Record](scope string, records map[string]T) (*View, error) {
	// Digest each record.
	view := &View{scope: scope, digests: make(map[string]uint64, len(records))}
	for key, record := range records {
		data, err := record.MarshalVT()
		if err != nil {
			return nil, err
		}
		view.digests[key] = xxhash.Sum64(data)
	}
	view.name()
	return view, nil
}

// name sets the View's version from its scope, keys and digests.
func (v *View) name() {
	// Compute the view's version from its scope, keys and digests, in key
	// order.
	scope := v.scope
	keys := slices.Sorted(func(yield func(string) bool) {
		for key := range v.digests {
			if !yield(key) {
				return
			}
		}
	})

	// Hash length-delimited names so scope/key boundaries cannot alias.
	sum := xxhash.New()
	var digest [8]byte
	binary.LittleEndian.PutUint64(digest[:], uint64(len(scope)))
	_, _ = sum.Write(digest[:])
	_, _ = sum.WriteString(scope)
	for _, key := range keys {
		binary.LittleEndian.PutUint64(digest[:], uint64(len(key)))
		_, _ = sum.Write(digest[:])
		_, _ = sum.WriteString(key)
		binary.LittleEndian.PutUint64(digest[:], v.digests[key])
		_, _ = sum.Write(digest[:])
	}
	v.Version = strconv.FormatUint(sum.Sum64(), 36)
}

// Since compares view with an earlier one, nil when unknown. An earlier view
// of another scope is unknown too.
func (v *View) Since(earlier *View) Changes {
	// An unknown reader takes everything.
	var changes Changes
	if earlier == nil || earlier.scope != v.scope {
		changes.Full = true
		for key := range v.digests {
			changes.Changed = append(changes.Changed, key)
		}
		slices.Sort(changes.Changed)
		return changes
	}

	// List what differs both ways.
	for key, digest := range v.digests {
		if held, ok := earlier.digests[key]; !ok || held != digest {
			changes.Changed = append(changes.Changed, key)
		}
	}
	for key := range earlier.digests {
		if _, ok := v.digests[key]; !ok {
			changes.Removed = append(changes.Removed, key)
		}
	}
	slices.Sort(changes.Changed)
	slices.Sort(changes.Removed)
	return changes
}
