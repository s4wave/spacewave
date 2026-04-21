package pagestore

import "testing"

func TestFreelistPageRoundTrip(t *testing.T) {
	buf := make([]byte, DefaultPageSize)
	ids := []PageID{3, 7, 11, 19}
	nextPage := PageID(23)

	written := EncodeFreelistPage(buf, nextPage, ids)
	if written != len(ids) {
		t.Fatalf("written=%d want=%d", written, len(ids))
	}

	gotNext, gotIDs, err := DecodeFreelistPage(buf)
	if err != nil {
		t.Fatalf("DecodeFreelistPage: %v", err)
	}
	if gotNext != nextPage {
		t.Fatalf("next page: got=%d want=%d", gotNext, nextPage)
	}
	if len(gotIDs) != len(ids) {
		t.Fatalf("id count: got=%d want=%d", len(gotIDs), len(ids))
	}
	for i := range ids {
		if gotIDs[i] != ids[i] {
			t.Fatalf("id %d: got=%d want=%d", i, gotIDs[i], ids[i])
		}
	}
}

func TestFreelistPageCapacity(t *testing.T) {
	capacity := FreelistPageCapacity(DefaultPageSize)
	if capacity <= 0 {
		t.Fatalf("capacity should be positive, got %d", capacity)
	}
}
