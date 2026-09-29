package sobject_world_engine

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/sirupsen/logrus"
)

// TestValidatorAdoptsCommitResultOnlyOnMatchingCacheKey proves the validator
// replay cache adopts the cached foreground commit result only when the base
// world root ref, the storage generation, and the operation bytes match the
// cache key. Any mismatch must fall through to processOp, so a stale cache
// entry can never be adopted onto the wrong base or the wrong operation.
func TestValidatorAdoptsCommitResultOnlyOnMatchingCacheKey(t *testing.T) {
	// Build the World roots.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	pid := newProcessTestPeerID(t)
	baseRootRef := mustBuildCommitCacheRef(t, "commit-cache/base-root")
	otherRootRef := mustBuildCommitCacheRef(t, "commit-cache/other-root")
	resultRootRef := mustBuildCommitCacheRef(t, "commit-cache/result-root")

	// Build two distinct operations.
	cachedOp := mustMarshalInitWorldOp(t, true)
	otherOp := mustMarshalInitWorldOp(t, false)
	if bytes.Equal(cachedOp, otherOp) {
		t.Fatal("cache-key test requires two distinct op encodings")
	}

	// Cache a result for the base root, generation 3, and cachedOp.
	cached := &commitResult{
		baseRootRef:       baseRootRef,
		storageGeneration: 3,
		opData:            cachedOp,
		resultRef:         &bucket.ObjectRef{RootRef: resultRootRef},
	}

	// Offer each operation on each base.
	for _, tc := range []struct {
		name       string
		stateRoot  *block.BlockRef
		generation uint64
		opData     []byte
		wantAdopt  bool
	}{
		{"matching base root and op bytes adopts", baseRootRef, 3, cachedOp, true},
		{"mismatched base root does not adopt", otherRootRef, 3, cachedOp, false},
		{"mismatched storage generation does not adopt", baseRootRef, 4, cachedOp, false},
		{"mismatched op bytes does not adopt", baseRootRef, 3, otherOp, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Replay the operation on the case's base.
			c := &Controller{le: le}
			c.lastCommitResult.Store(cached)
			so := &commitCacheValidatorSharedObject{
				currentStateData: mustMarshalInnerStateHead(t, tc.stateRoot, tc.generation),
				ops: []*sobject.SOOperationInner{{
					PeerId: pid.String(),
					Nonce:  1,
					OpData: tc.opData,
				}},
			}
			if err := c.executeProcessOpsAsValidator(ctx, so); err != nil {
				t.Fatalf("validator returned error: %v", err)
			}
			if len(so.opResults) != 1 {
				t.Fatalf("expected 1 op result, got %d", len(so.opResults))
			}

			// A match adopts the cached head on the same generation.
			if tc.wantAdopt {
				if so.nextStateData == nil {
					t.Fatal("matching cache key must adopt the cached commit result")
				}
				adopted := &InnerState{}
				if err := adopted.UnmarshalVT(*so.nextStateData); err != nil {
					t.Fatalf("unmarshal adopted state: %v", err)
				}
				if !adopted.GetHeadRef().GetRootRef().EqualsRef(resultRootRef) {
					t.Fatal("adopted state must carry the cached result head ref")
				}
				if adopted.GetStorageGeneration() != tc.generation {
					t.Fatal("adopted state must keep the storage generation")
				}
				if !so.opResults[0].GetSuccess() {
					t.Fatal("adopted op must report success")
				}
				return
			}

			// A mismatch falls through to processOp, which rejects an
			// InitWorld on an initialized World.
			if so.nextStateData != nil {
				t.Fatal("mismatched cache key must not adopt the cached commit result")
			}
			if so.opResults[0].GetSuccess() {
				t.Fatal("non-adopted op must be resolved by processOp, not silently accepted from cache")
			}
		})
	}
}

// commitCacheValidatorSharedObject drives executeProcessOpsAsValidator by
// invoking its callback with controlled state and ops, capturing the result.
type commitCacheValidatorSharedObject struct {
	testSharedObject
	// currentStateData is the accepted World before validator replay.
	currentStateData []byte
	// ops contains operations offered to the validator.
	ops []*sobject.SOOperationInner

	// nextStateData captures the validator's proposed World.
	nextStateData *[]byte
	// opResults captures acceptance or rejection for each operation.
	opResults []*sobject.SOOperationResult
}

// ProcessOperations runs one controlled validator batch.
func (s *commitCacheValidatorSharedObject) ProcessOperations(ctx context.Context, watch bool, cb sobject.ProcessOpsFunc) error {
	// Admit each op's peer and run the callback once.
	var err error
	snapshot := &testSharedObjectSnapshot{participants: make(map[string]*sobject.SOParticipantConfig)}
	for _, op := range s.ops {
		snapshot.participants[op.GetPeerId()] = &sobject.SOParticipantConfig{PeerId: op.GetPeerId()}
	}
	s.nextStateData, s.opResults, err = cb(ctx, snapshot, s.currentStateData, s.ops)
	return err
}

// mustBuildCommitCacheRef derives a reproducible cache key from seed.
func mustBuildCommitCacheRef(t *testing.T, seed string) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(seed), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

// mustMarshalInnerStateHead encodes a World head for validator replay.
func mustMarshalInnerStateHead(t *testing.T, root *block.BlockRef, generation uint64) []byte {
	t.Helper()
	data, err := (&InnerState{
		HeadRef:           &bucket.ObjectRef{RootRef: root},
		StorageGeneration: generation,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	return data
}

// mustMarshalInitWorldOp builds distinct operation encodings for cache-key checks.
func mustMarshalInitWorldOp(t *testing.T, lastChangeDisable bool) []byte {
	t.Helper()
	data, err := (&SOWorldOp{
		Body: &SOWorldOp_InitWorld{
			InitWorld: &InitWorldOp{LastChangeDisable: lastChangeDisable},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	return data
}
