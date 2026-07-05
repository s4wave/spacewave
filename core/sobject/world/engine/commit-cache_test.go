package sobject_world_engine

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	alpha_testbed "github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// TestValidatorAdoptsCommitResultOnlyOnMatchingCacheKey proves the validator
// replay cache adopts the cached foreground commit result only when both the
// base world root ref and the operation bytes match the cache key. A mismatched
// base root or mismatched op bytes must fall through to processOp, so a stale
// cache entry can never be adopted onto the wrong base or the wrong operation.
func TestValidatorAdoptsCommitResultOnlyOnMatchingCacheKey(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	pid := newProcessTestPeerID(t)

	baseRootRef := mustBuildCommitCacheRef(t, "commit-cache/base-root")
	otherRootRef := mustBuildCommitCacheRef(t, "commit-cache/other-root")
	resultRootRef := mustBuildCommitCacheRef(t, "commit-cache/result-root")

	cachedOp := mustMarshalInitWorldOp(t, true)
	otherOp := mustMarshalInitWorldOp(t, false)
	if bytes.Equal(cachedOp, otherOp) {
		t.Fatal("cache-key test requires two distinct op encodings")
	}

	cached := &commitResult{
		baseRootRef: baseRootRef,
		opData:      cachedOp,
		resultState: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: resultRootRef}},
	}

	for _, tc := range []struct {
		name      string
		stateRoot *block.BlockRef
		opData    []byte
		wantAdopt bool
	}{
		{"matching base root and op bytes adopts", baseRootRef, cachedOp, true},
		{"mismatched base root does not adopt", otherRootRef, cachedOp, false},
		{"mismatched op bytes does not adopt", baseRootRef, otherOp, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Controller{le: le}
			c.lastCommitResult.Store(cached)

			so := &commitCacheValidatorSharedObject{
				currentStateData: mustMarshalInnerStateHead(t, tc.stateRoot),
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
				if !so.opResults[0].GetSuccess() {
					t.Fatal("adopted op must report success")
				}
				return
			}

			if so.nextStateData != nil {
				t.Fatal("mismatched cache key must not adopt the cached commit result")
			}
			if so.opResults[0].GetSuccess() {
				t.Fatal("non-adopted op must be resolved by processOp, not silently accepted from cache")
			}
		})
	}
}

// TestWatchStateLocalQueueAdoptsCommitResultOnlyOnMatchingCacheKey proves the
// watch-state local-queue replay cache adopts the cached commit result only when
// the base world root ref and the queued op bytes match the cache key. A
// matching key skips reprocessing the op; a mismatched base root or op bytes
// reprocesses the queued op, which surfaces the decode error of the deliberately
// malformed op used here as proof that the cache guard did not short-circuit it.
func TestWatchStateLocalQueueAdoptsCommitResultOnlyOnMatchingCacheKey(t *testing.T) {
	ctx := context.Background()
	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Deliberately malformed SOWorldOp wire bytes: an incomplete varint tag that
	// fails UnmarshalVT. When the queued op is reprocessed (not adopted), the
	// watch-state loop returns this decode error; when it is adopted, the op is
	// skipped and no error surfaces.
	malformedOpA := []byte{0xff}
	malformedOpB := []byte{0xfe}

	for _, tc := range []struct {
		name      string
		matchRoot bool
		queuedOp  []byte
		cachedOp  []byte
		wantAdopt bool
	}{
		{"matching base root and op bytes adopts and skips reprocessing", true, malformedOpA, malformedOpA, true},
		{"mismatched base root reprocesses op", false, malformedOpA, malformedOpA, false},
		{"mismatched op bytes reprocesses op", true, malformedOpB, malformedOpA, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ocs, err := tb.BuildEmptyCursor(ctx)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer ocs.Release()

			bengine, err := world_block.NewEngine(ctx, tb.Logger, ocs, world_mock.LookupMockOp, nil, false)
			if err != nil {
				t.Fatal(err.Error())
			}

			headRef := bengine.GetRootRef().CloneVT()
			headRef.BucketId = ""
			stateData, err := (&InnerState{HeadRef: headRef}).MarshalVT()
			if err != nil {
				t.Fatal(err.Error())
			}

			cachedRoot := headRef.GetRootRef()
			if !tc.matchRoot {
				cachedRoot = mustBuildCommitCacheRef(t, "commit-cache/watch-mismatch/"+tc.name)
			}

			c := &Controller{le: tb.Logger}
			c.lastCommitResult.Store(&commitResult{
				baseRootRef: cachedRoot,
				opData:      tc.cachedOp,
				resultState: &InnerState{HeadRef: headRef.CloneVT()},
			})

			so := &testSharedObject{blockStore: newTestBlockStore(tb.EngineBucketID, tb.Volume)}
			soEngine := &soEngine{c: c, so: so, bengine: bengine}
			snap := &commitCacheLocalQueueSnapshot{
				testSharedObjectSnapshot: testSharedObjectSnapshot{
					rootInner: &sobject.SORootInner{Seqno: 1, StateData: stateData},
				},
				localOpQueue: []*sobject.QueuedSOOperation{{
					LocalId: "commit-cache-op",
					OpData:  tc.queuedOp,
				}},
			}

			err = c.executeWatchSOStateOnce(ctx, tb.Logger, so, snap, soEngine)
			if tc.wantAdopt {
				if err != nil {
					t.Fatalf("matching cache key must adopt and skip reprocessing, got error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("mismatched cache key must reprocess the queued op and surface its decode error")
			}
		})
	}
}

// commitCacheValidatorSharedObject drives executeProcessOpsAsValidator by
// invoking its callback with controlled state and ops, capturing the result.
type commitCacheValidatorSharedObject struct {
	testSharedObject
	currentStateData []byte
	ops              []*sobject.SOOperationInner

	nextStateData *[]byte
	opResults     []*sobject.SOOperationResult
}

func (s *commitCacheValidatorSharedObject) ProcessOperations(ctx context.Context, watch bool, cb sobject.ProcessOpsFunc) error {
	var err error
	s.nextStateData, s.opResults, err = cb(ctx, nil, s.currentStateData, s.ops)
	return err
}

// commitCacheLocalQueueSnapshot serves a local op queue to executeWatchSOStateOnce.
type commitCacheLocalQueueSnapshot struct {
	testSharedObjectSnapshot
	localOpQueue []*sobject.QueuedSOOperation
}

func (s *commitCacheLocalQueueSnapshot) GetOpQueue(ctx context.Context) ([]*sobject.SOOperation, []*sobject.QueuedSOOperation, error) {
	return nil, s.localOpQueue, nil
}

func mustBuildCommitCacheRef(t *testing.T, seed string) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(seed), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

func mustMarshalInnerStateHead(t *testing.T, root *block.BlockRef) []byte {
	t.Helper()
	data, err := (&InnerState{HeadRef: &bucket.ObjectRef{RootRef: root}}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	return data
}

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
