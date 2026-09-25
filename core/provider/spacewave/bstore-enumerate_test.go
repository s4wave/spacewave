package provider_spacewave

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// enumerateTestPack packs datas and returns the pack bytes and its hashes.
func enumerateTestPack(t *testing.T, datas ...string) ([]byte, []string) {
	t.Helper()
	var buf bytes.Buffer
	var keys []string
	idx := 0
	_, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
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

	var mtx sync.Mutex
	var opened []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/pull") {
			_, _ = w.Write(pullData)
			return
		}
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		data, ok := packs[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mtx.Lock()
		opened = append(opened, id)
		mtx.Unlock()
		http.ServeContent(w, r, id, time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	refs, err := acc.EnumerateBlockRefs(t.Context(), "bstore-1")
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}

	got := make([]string, len(refs))
	for i, ref := range refs {
		got[i] = ref.GetHash().MarshalString()
	}
	slices.Sort(got)
	want := slices.Compact(slices.Sorted(slices.Values(append(keys1, keys2...))))
	if !slices.Equal(got, want) {
		t.Fatalf("refs = %v, want %v", got, want)
	}
	mtx.Lock()
	defer mtx.Unlock()
	for _, id := range opened {
		if id == "superseded" || id == "replaced" {
			t.Fatalf("enumeration read inactive pack %s", id)
		}
	}
}
