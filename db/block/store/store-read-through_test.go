package block_store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/net/hash"
)

func newReadThroughTestBlockStore() *block_store_kvtx.KVTxBlock {
	return block_store_kvtx.NewKVTxBlock(
		store_kvkey.NewDefaultKVKey(),
		hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()),
		hash.HashType_HashType_BLAKE3,
		false,
	)
}

// refsTestStore records the refs of each put and serves them with the bytes.
type refsTestStore struct {
	block.StoreOps

	mu   sync.Mutex
	refs map[string][]*block.BlockRef
}

func newRefsTestStore() *refsTestStore {
	return &refsTestStore{
		StoreOps: newReadThroughTestBlockStore(),
		refs:     make(map[string][]*block.BlockRef),
	}
}

func (s *refsTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Store the block bytes before recording their references.
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}

	// Retain the block references for later stored block reads.
	s.mu.Lock()
	s.refs[ref.MarshalString()] = opts.GetRefs()
	s.mu.Unlock()
	return ref, existed, nil
}

func (s *refsTestStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

func (s *refsTestStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.StoredBlock{Data: data, Refs: s.getRefs(ref), RefsKnown: true}, nil
}

func (s *refsTestStore) getRefs(ref *block.BlockRef) []*block.BlockRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refs[ref.MarshalString()]
}

func TestStoreReadThroughWritebackUsesWritablePrimary(t *testing.T) {
	// Create primary and lower stores with a child block in the lower store.
	ctx := context.Background()
	primary := newRefsTestStore()
	lower := newRefsTestStore()
	child, _, err := lower.PutBlock(ctx, []byte("child"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Store a lower block whose references must survive writeback.
	data := []byte("read-through writeback")
	ref, _, err := lower.PutBlock(ctx, data, &block.PutOpts{Refs: []*block.BlockRef{child}})
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the primary store starts without the lower block.
	if _, found, err := primary.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("primary before read found=%v err=%v, want absent", found, err)
	}

	// Open a read-through scope with writeback enabled.
	var lowerSource block.StoreOps = lower
	store := block_store.NewStoreReadThrough(
		func() block.StoreOps { return primary },
		func() block.StoreOps { return lowerSource },
		block_store.CacheFill,
	)
	scoped, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the lower block through the scope and release the read operation.
	got, found, err := scoped.GetBlock(ctx, ref)
	release()
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("scoped lower read = %q/%v/%v", got, found, err)
	}

	// Wait for the background fill, then verify that writeback preserves the
	// block bytes and refs without the lower source.
	lowerSource = nil
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, found, err = primary.GetBlock(ctx, ref)
		if err != nil || found || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("primary writeback = %q/%v/%v", got, found, err)
	}
	refs := primary.getRefs(ref)
	if len(refs) != 1 || !refs[0].EqualsRef(child) {
		t.Fatalf("primary writeback refs = %v, want [%v]", refs, child)
	}
}

func TestStoreReadThroughByteOnlySourceSkipsFill(t *testing.T) {
	// Store a lower block in a source that does not retain references.
	ctx := context.Background()
	primary := newRefsTestStore()
	lower := newReadThroughTestBlockStore()
	data := []byte("byte-only source")
	ref, _, err := lower.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that read-through serves bytes from the lower source.
	store := block_store.NewStoreReadThrough(
		func() block.StoreOps { return primary },
		func() block.StoreOps { return lower },
		block_store.CacheFill,
	)
	got, found, err := store.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("lower read = %q/%v/%v", got, found, err)
	}

	// Verify that the stored block reports its references as unknown.
	stored, err := store.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil || stored.RefsKnown {
		t.Fatalf("GetStoredBlock = %v/%v, want bytes without refs", stored, err)
	}

	// Verify that the byte-only lower hit leaves the primary store empty.
	if _, found, err := primary.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("primary after byte-only read found=%v err=%v, want absent", found, err)
	}
}

// TestStoreReadThroughAbsentLowerIsUnavailable tests that a primary miss while
// the lower source resolves to nil reports the block unavailable to a read
// that may not wait for a peer, recording it, and a miss to any other read.
func TestStoreReadThroughAbsentLowerIsUnavailable(t *testing.T) {
	// A pipeline whose lower source is down, reading a block the primary
	// lacks.
	ctx := t.Context()
	primary := newRefsTestStore()
	store := block_store.NewStoreReadThrough(
		func() block.StoreOps { return primary },
		func() block.StoreOps { return nil },
		block_store.CacheFill,
	)
	ref, err := block.BuildBlockRef([]byte("held by a peer"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// A read that may wait reports a miss, as before.
	if _, found, err := store.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("GetBlock found=%v err=%v, want a miss", found, err)
	}
	if stored, err := store.GetStoredBlock(ctx, ref); err != nil || stored != nil {
		t.Fatalf("GetStoredBlock = %v/%v, want a miss", stored, err)
	}

	// A read that may not wait reports the block unavailable and records it.
	noWait, peerWait := block.WithoutPeerWait(ctx)
	if _, _, err := store.GetBlock(noWait, ref); !errors.Is(err, block.ErrUnavailable) {
		t.Fatalf("GetBlock err=%v, want ErrUnavailable", err)
	}
	if _, err := store.GetStoredBlock(noWait, ref); !errors.Is(err, block.ErrUnavailable) {
		t.Fatalf("GetStoredBlock err=%v, want ErrUnavailable", err)
	}
	if !peerWait.GetRef().EqualsRef(ref) {
		t.Fatalf("recorded %v, want %v", peerWait.GetRef(), ref)
	}
}

// answerTestStore answers every read with one scripted answer and counts the
// reads it served.
type answerTestStore struct {
	block.StoreOps

	// answer is the scripted answer: errAnswerHit, errAnswerMiss, or another error.
	answer error
	// reads counts the reads the store answered.
	reads int
}

// errAnswerHit and errAnswerMiss script a hit and a plain miss.
var (
	errAnswerHit  = errors.New("hit")
	errAnswerMiss = errors.New("miss")
)

func (s *answerTestStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	if stored == nil {
		return nil, false, err
	}
	return stored.Data, true, err
}

func (s *answerTestStore) GetStoredBlock(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Count the read, then give the scripted answer.
	s.reads++
	switch s.answer {
	case errAnswerHit:
		return &block.StoredBlock{Data: []byte("held")}, nil
	case errAnswerMiss:
		return nil, nil
	}
	return nil, s.answer
}

func (s *answerTestStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	return stored != nil, err
}

func (s *answerTestStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Answer every ref alike, failing the whole batch on an error.
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// TestStoreReadThroughCombinesSources tests that a read takes a hit from
// either source, that a source that failed or could not answer does not keep
// the read from the other, and that a read neither source served reports a
// failure first, then an unavailable source, and a miss only when both sources
// said they lack the block.
func TestStoreReadThroughCombinesSources(t *testing.T) {
	// Script each source's answer and the outcome of the read.
	unavailable := func(ref *block.BlockRef) error {
		return &block.UnavailableError{Ref: ref, Holders: []string{"peer"}}
	}
	failed := errors.New("backend down")
	cases := []struct {
		name           string
		primary, lower func(*block.BlockRef) error
		want           string
	}{
		{"primary hit", answerWith(errAnswerHit), answerWith(errAnswerMiss), "hit"},
		{"unavailable primary, lower hit", unavailable, answerWith(errAnswerHit), "hit"},
		{"failed primary, lower hit", answerWith(failed), answerWith(errAnswerHit), "hit"},
		{"not found primary, lower hit", answerWith(block.ErrNotFound), answerWith(errAnswerHit), "hit"},
		{"both miss", answerWith(errAnswerMiss), answerWith(errAnswerMiss), "miss"},
		{"not found primary, lower miss", answerWith(block.ErrNotFound), answerWith(errAnswerMiss), "miss"},
		{"unavailable primary, lower miss", unavailable, answerWith(errAnswerMiss), "unavailable"},
		{"primary miss, unavailable lower", answerWith(errAnswerMiss), unavailable, "unavailable"},
		{"not found primary, unavailable lower", answerWith(block.ErrNotFound), unavailable, "unavailable"},
		{"failed primary, unavailable lower", answerWith(failed), unavailable, "failed"},
		{"unavailable primary, failed lower", unavailable, answerWith(failed), "failed"},
	}

	// Run every case through every read, with and without a fill.
	for _, tc := range cases {
		for _, fill := range []block_store.Fill{nil, block_store.CacheFill} {
			for _, read := range combineTestReads {
				t.Run(tc.name+"/"+read.name, func(t *testing.T) {
					// Build the sources and read a block through them.
					ref, err := block.BuildBlockRef([]byte(tc.name), nil)
					if err != nil {
						t.Fatal(err)
					}
					primary := &answerTestStore{StoreOps: newReadThroughTestBlockStore(), answer: tc.primary(ref)}
					lower := &answerTestStore{StoreOps: newReadThroughTestBlockStore(), answer: tc.lower(ref)}
					store := block_store.NewStoreReadThrough(
						func() block.StoreOps { return primary },
						func() block.StoreOps { return lower },
						fill,
					)
					found, err := read.read(t.Context(), store, ref)

					// Require the outcome, and no lower read after a primary hit.
					if got := combineTestOutcome(found, err); got != tc.want {
						t.Fatalf("read = found %v, %v (%s); want %s", found, err, got, tc.want)
					}
					if tc.want == "hit" && primary.answer == errAnswerHit && lower.reads != 0 {
						t.Fatalf("lower read %d times after a primary hit", lower.reads)
					}
				})
			}
		}
	}
}

// TestStoreReadThroughEndedReadStops tests that a read whose context ended
// returns the primary's answer without asking the lower source.
func TestStoreReadThroughEndedReadStops(t *testing.T) {
	// End the read, and have the primary report it.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ref, err := block.BuildBlockRef([]byte("ended"), nil)
	if err != nil {
		t.Fatal(err)
	}
	primary := &answerTestStore{StoreOps: newReadThroughTestBlockStore(), answer: context.Canceled}
	lower := &answerTestStore{StoreOps: newReadThroughTestBlockStore(), answer: errAnswerHit}
	store := block_store.NewStoreReadThrough(
		func() block.StoreOps { return primary },
		func() block.StoreOps { return lower },
		block_store.CacheFill,
	)

	// Every read stops with the primary's answer.
	for _, read := range combineTestReads {
		if found, err := read.read(ctx, store, ref); found || !errors.Is(err, context.Canceled) {
			t.Fatalf("%s = found %v, %v; want canceled", read.name, found, err)
		}
	}
	if lower.reads != 0 {
		t.Fatalf("lower read %d times after the read ended", lower.reads)
	}
}

// answerWith returns a script giving ans for any ref.
func answerWith(ans error) func(*block.BlockRef) error {
	return func(*block.BlockRef) error { return ans }
}

// combineTestReads are the reads that combine the sources' answers, each
// reporting whether it found the block.
var combineTestReads = []struct {
	name string
	read func(context.Context, *block_store.StoreReadThrough, *block.BlockRef) (bool, error)
}{
	{"GetBlock", func(ctx context.Context, s *block_store.StoreReadThrough, ref *block.BlockRef) (bool, error) {
		_, found, err := s.GetBlock(ctx, ref)
		return found, err
	}},
	{"GetStoredBlock", func(ctx context.Context, s *block_store.StoreReadThrough, ref *block.BlockRef) (bool, error) {
		stored, err := s.GetStoredBlock(ctx, ref)
		return stored != nil, err
	}},
	{"GetBlockExists", func(ctx context.Context, s *block_store.StoreReadThrough, ref *block.BlockRef) (bool, error) {
		return s.GetBlockExists(ctx, ref)
	}},
	{"GetBlockExistsBatch", func(ctx context.Context, s *block_store.StoreReadThrough, ref *block.BlockRef) (bool, error) {
		found, err := s.GetBlockExistsBatch(ctx, []*block.BlockRef{ref})
		return len(found) == 1 && found[0], err
	}},
}

// combineTestOutcome classifies a read's result as a hit, a miss, an
// unavailable block, or a failure.
func combineTestOutcome(found bool, err error) string {
	switch {
	case found && err == nil:
		return "hit"
	case found:
		return "hit with an error"
	case errors.Is(err, block.ErrUnavailable) && !errors.Is(err, block.ErrNotFound):
		return "unavailable"
	case err == nil || errors.Is(err, block.ErrNotFound) && !errors.Is(err, block.ErrUnavailable):
		return "miss"
	}
	return "failed"
}
