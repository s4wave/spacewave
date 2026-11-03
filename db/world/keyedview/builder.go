package keyedview

import "github.com/cespare/xxhash/v2"

// Builder builds the successive Views of one keyed view, digesting only the
// records that are not the same value as in the last build. A view read from
// decoded records it keeps, as most World views are, changes few of them per
// revision, so a large view's next version costs a hash per changed record.
// Records must be immutable once built, as shared results are.
type Builder struct {
	// records holds the last build's records by key.
	records map[string]Record
	// digests holds the last build's digests by key.
	digests map[string]uint64
}

// Build digests records of scope into a View, reusing the last build's digest
// of each record that is the same value. The records' dynamic types must be
// comparable, such as message pointers.
func (b *Builder) Build(scope string, records map[string]Record) (*View, error) {
	// Digest each record not built last time.
	view := &View{scope: scope, digests: make(map[string]uint64, len(records))}
	for key, record := range records {
		if last, ok := b.records[key]; ok && last == record {
			view.digests[key] = b.digests[key]
			continue
		}
		data, err := record.MarshalVT()
		if err != nil {
			return nil, err
		}
		view.digests[key] = xxhash.Sum64(data)
	}
	b.records, b.digests = records, view.digests
	view.name()
	return view, nil
}
