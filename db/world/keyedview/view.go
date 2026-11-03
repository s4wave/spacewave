package keyedview

import (
	"encoding/binary"
	"slices"
	"strconv"

	"github.com/cespare/xxhash/v2"
)

// View is one result of a keyed view: each record's digest by key, and the
// version that names them all. A View is immutable once built.
type View struct {
	// Version names the result's scope and content; equal results share it.
	Version string
	// scope names what the records belong to; views of different scopes share
	// no records.
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
	// Name the view by its scope, keys and digests, in key order.
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
