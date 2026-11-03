package keyedview

import (
	"strconv"
	"testing"
)

// countedRecord is a record that counts its encodings.
type countedRecord struct {
	// data is the record's encoding.
	data string
	// encodes counts the encodings of every record sharing it.
	encodes *int
}

// MarshalVT counts the encoding and returns the data.
func (r *countedRecord) MarshalVT() ([]byte, error) {
	*r.encodes++
	return []byte(r.data), nil
}

// TestBuilderHashesChangedRecords rebuilds a large view after one record
// changes and sees the Builder encode that one record, while naming the same
// version Build names.
func TestBuilderHashesChangedRecords(t *testing.T) {
	// Build a view of a thousand records.
	var encodes int
	records := make(map[string]Record, 1000)
	for i := range 1000 {
		records[strconv.Itoa(i)] = &countedRecord{data: "record " + strconv.Itoa(i), encodes: &encodes}
	}
	var builder Builder
	first, err := builder.Build("scope", records)
	if err != nil {
		t.Fatal(err)
	}
	if encodes != 1000 {
		t.Fatalf("the first build encoded %d records, want 1000", encodes)
	}

	// Replace one record and rebuild.
	next := make(map[string]Record, len(records))
	for key, record := range records {
		next[key] = record
	}
	next["7"] = &countedRecord{data: "record 7 changed", encodes: &encodes}
	encodes = 0
	second, err := builder.Build("scope", next)
	if err != nil {
		t.Fatal(err)
	}
	if encodes != 1 {
		t.Fatalf("the rebuild encoded %d records, want the one changed", encodes)
	}

	// The version and the changes match a full build's.
	full, err := Build("scope", next)
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != full.Version {
		t.Fatalf("the rebuild named version %s, a full build %s", second.Version, full.Version)
	}
	changes := second.Since(first)
	if changes.Full || len(changes.Changed) != 1 || changes.Changed[0] != "7" || len(changes.Removed) != 0 {
		t.Fatalf("the rebuild's changes are %+v, want record 7", changes)
	}

	// History resumes record removals and falls back when the scope changes.
	history := NewHistory(2)
	history.Add(first)
	delete(next, "8")
	third, err := Build("scope", next)
	if err != nil {
		t.Fatal(err)
	}
	delta := history.Since(first.Version, third)
	if delta.Full || len(delta.Removed) != 1 || delta.Removed[0] != "8" || len(delta.Changed) != 1 || delta.Changed[0] != "7" {
		t.Fatalf("resumed changes: %+v", delta)
	}
	other, err := Build("other-scope", next)
	if err != nil {
		t.Fatal(err)
	}
	if changes := history.Since(third.Version, other); !changes.Full {
		t.Fatal("a different scope reused the previous records")
	}
	if changes := history.Since("forgotten", third); !changes.Full {
		t.Fatal("an unknown version omitted a full replacement")
	}

	// Distinct scope/key boundaries cannot share one resume version.
	one, err := Build("a", map[string]*countedRecord{"bc": {data: "same", encodes: &encodes}})
	if err != nil {
		t.Fatal(err)
	}
	two, err := Build("ab", map[string]*countedRecord{"c": {data: "same", encodes: &encodes}})
	if err != nil {
		t.Fatal(err)
	}
	if one.Version == two.Version {
		t.Fatal("scope/key boundary alias reused a version")
	}
}
