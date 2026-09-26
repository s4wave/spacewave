package node_controller

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/sirupsen/logrus"
)

// TestBucketLookupHandleReleased checks that a lookup wait through a handle
// ends when the bucket run that published the handle exits, instead of waiting
// for a lookup that no run will provide.
func TestBucketLookupHandleReleased(t *testing.T) {
	ctx := t.Context()
	c := &Controller{le: logrus.NewEntry(logrus.New()), cc: &Config{}}
	_, lb := c.newLoadedBucket("test-bucket")

	// A disabled lookup keeps the lookup empty, so GetLookup must wait.
	lb.bucketConf = &bucket.Config{
		Id:     "test-bucket",
		Rev:    1,
		Lookup: &bucket.LookupConfig{Disable: true},
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	runErr := make(chan error, 1)
	go func() { runErr <- lb.execute(runCtx) }()
	st, err := lb.stateCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newBucketLookupHandle(lb, st)

	lookupErr := make(chan error, 1)
	go func() {
		_, err := h.GetLookup(ctx)
		lookupErr <- err
	}()
	cancelRun()
	<-runErr

	select {
	case err := <-lookupErr:
		if !errors.Is(err, bucket.ErrBucketNotFound) {
			t.Fatalf("GetLookup returned %v, want ErrBucketNotFound", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("GetLookup still waiting after the bucket run exited")
	}
	if !h.GetDisposed() {
		t.Fatal("handle not disposed after the bucket run exited")
	}
}
