package block_store_s3

import (
	"testing"

	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// TestReadOnlyIndexWritesNothing builds a pack store with ReadOnlyIndex over
// an index that refuses every write: it runs no indexer, and a listed pack, a
// kept tail and a removed pack write nothing, while the listed pack stays
// known in memory.
func TestReadOnlyIndexWritesNothing(t *testing.T) {
	ctx := t.Context()
	index := &stopAfterWrites{Store: hashmap.NewHashmapKvtx(hashmap.NewBTreeMap[[]byte]()), writes: 0}
	s := NewPackStore(logrus.NewEntry(logrus.New()), nil, "bucket", "prefix/", &PackStoreOptions{Index: index, ReadOnlyIndex: true})
	defer s.Close()
	if s.indexer != nil {
		t.Fatal("read-only index runs an indexer")
	}

	// List a pack, keep a tail and remove a pack.
	if err := s.index.addListed(ctx, &packfile.PackfileEntry{Id: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.index.flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if _, ok := s.index.rows["a"]; !ok {
		t.Fatal("listed pack not kept in memory")
	}
	if err := s.index.Set(ctx, "a", []byte("tail")); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.index.queuedTail("a"); ok {
		t.Fatal("read-only index queued a tail for its write")
	}
	if err := s.index.remove(ctx, []string{"a"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if index.opened != 0 {
		t.Fatalf("read-only index opened %d write transactions", index.opened)
	}
}
