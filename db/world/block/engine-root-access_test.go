package world_block

import (
	"context"
	"runtime"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

func TestEngineAccessWorldStateSameBucketConcurrentClose(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("requires concurrent execution")
	}

	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Release()

	const rounds = 32
	const workers = 16
	for range rounds {
		eng, err := NewEngine(ctx, le, base, nil, nil, false)
		if err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		entered := make(chan struct{})
		var enteredOnce sync.Once
		var wg sync.WaitGroup
		var panicMu sync.Mutex
		var panics []any
		for range workers {
			wg.Go(func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						panicMu.Lock()
						panics = append(panics, recovered)
						panicMu.Unlock()
					}
				}()
				<-start
				for {
					err := eng.AccessWorldState(ctx, nil, func(*bucket_lookup.Cursor) error {
						enteredOnce.Do(func() { close(entered) })
						return nil
					})
					if err == ErrEngineClosed {
						return
					}
					if err != nil {
						t.Errorf("access failed: %v", err)
						return
					}
				}
			})
		}
		close(start)
		<-entered
		if err := eng.Close(); err != nil {
			t.Fatal(err)
		}
		wg.Wait()

		panicMu.Lock()
		panicsFound := append([]any(nil), panics...)
		panicMu.Unlock()
		if len(panicsFound) != 0 {
			t.Fatalf("concurrent close caused %d panic(s), first: %v", len(panicsFound), panicsFound[0])
		}
	}
}

func TestEngineAccessWorldStateReferencesAndCallbackBoundary(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Release()

	eng, err := NewEngine(ctx, le, base, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	testAccess := func(ref *bucket.ObjectRef) {
		t.Helper()
		if err := eng.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
			if !eng.rmtx.TryLock() {
				t.Fatal("callback ran while Engine.rmtx was held")
			}
			eng.rmtx.Unlock()
			if cursor == nil {
				t.Fatal("callback received nil cursor")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	testAccess(nil)
	testAccess(&bucket.ObjectRef{})
}
