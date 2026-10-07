package provider_spacewave

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// enumerateTestPack packs datas and returns the pack bytes and its hashes.
func enumerateTestPack(t *testing.T, datas ...string) ([]byte, []string) {
	// Pack each payload and retain its hash for the expected enumeration.
	t.Helper()
	var buf bytes.Buffer
	keys := make([]string, 0, len(datas))
	idx := 0
	_, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		// Stop at the end, otherwise hash and emit the next block.
		if idx >= len(datas) {
			return nil, nil, nil
		}
		data := []byte(datas[idx])
		idx++
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			return nil, nil, err
		}
		keys = append(keys, h.MarshalString())
		return h, &block.StoredBlock{Data: data, RefsKnown: true}, nil
	})
	if err != nil {
		t.Fatalf("pack %v: %v", datas, err)
	}
	return buf.Bytes(), keys
}

// TestEnumerateBlockRefs verifies enumeration lists each block of the current
// packs once and skips superseded and replaced packs.
func TestEnumerateBlockRefs(t *testing.T) {
	// Pack two current packs sharing a block, and two inactive packs.
	current1, keys1 := enumerateTestPack(t, "alpha", "shared")
	current2, keys2 := enumerateTestPack(t, "bravo", "shared")
	superseded, _ := enumerateTestPack(t, "superseded")
	replaced, _ := enumerateTestPack(t, "replaced")
	packs := map[string][]byte{
		"current-1":  current1,
		"current-2":  current2,
		"superseded": superseded,
		"replaced":   replaced,
	}

	// List every pack, superseding one and replacing another.
	entry := func(id string) *packfile.PackfileEntry {
		return &packfile.PackfileEntry{Id: id, SizeBytes: uint64(len(packs[id]))}
	}
	pull := &packfile.PullResponse{
		Entries: []*packfile.PackfileEntry{
			entry("current-1"),
			entry("current-2"),
			entry("superseded"),
			entry("replaced"),
		},
		ReplacementEvents: []*packfile.PackReplacementEvent{{ReplacedPackIds: []string{"replaced"}}},
	}
	pull.Entries[2].SupersededBy = "current-2"
	pullData, err := pull.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Serve the pull.
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.Handle("GET /api/bstore/{id}/sync/pull", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(pullData)
	}))

	// Grant a read URL for each requested pack.
	mux.Handle("POST /api/bstore/{id}/read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decode the requested packs.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		req := &packfile.ReadRequest{}
		if err := req.UnmarshalVT(body); err != nil {
			t.Error(err)
			return
		}

		// Answer one grant per pack.
		resp := &packfile.ReadResponse{}
		for _, id := range req.GetPackIds() {
			resp.Grants = append(resp.Grants, &packfile.ReadGrant{
				PackId:    id,
				Url:       srv.URL + "/pack/" + id,
				ExpiresAt: timestamppb.New(time.Now().Add(10 * time.Minute)),
			})
		}
		data, err := resp.MarshalVT()
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = w.Write(data)
	}))

	// Serve granted range reads, recording each pack opened.
	var mtx sync.Mutex
	var opened []string
	mux.Handle("GET /pack/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Find the requested pack.
		id := r.PathValue("id")
		data, ok := packs[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Record and serve the range read.
		mtx.Lock()
		opened = append(opened, id)
		mtx.Unlock()
		http.ServeContent(w, r, id, time.Time{}, bytes.NewReader(data))
	}))

	// Enumerate the block store.
	acc := NewTestProviderAccount(t, srv.URL)
	refs, err := acc.EnumerateBlockRefs(t.Context(), "bstore-1")
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}

	// Expect each block of the current packs once.
	got := make([]string, len(refs))
	for i, ref := range refs {
		got[i] = ref.GetHash().MarshalString()
	}
	slices.Sort(got)
	want := slices.Compact(slices.Sorted(slices.Values(append(keys1, keys2...))))
	if !slices.Equal(got, want) {
		t.Fatalf("refs = %v, want %v", got, want)
	}

	// Expect no read of an inactive pack.
	mtx.Lock()
	defer mtx.Unlock()
	for _, id := range opened {
		if id == "superseded" || id == "replaced" {
			t.Fatalf("enumeration read inactive pack %s", id)
		}
	}
}
