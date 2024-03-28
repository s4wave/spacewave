package volume_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// This file contains some limited tests.
// Other volume-specific e2e tests are done elsewhere.

// TestBucketHandleFlush tests looking for a bucket, not finding it, then
// pushing it with a ApplyBucketConfig: we then expect the controller to
// re-check for the configuration, find it, and create new handles.
func TestBucketHandleFlush(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	bucketID := "test-bucket-flush"
	b := tb.Bus
	vol := tb.Volume
	volumeID := vol.GetID()
	// try to BuildBucketAPI
	vals := make(chan bucket.BuildBucketAPIValue)
	_, bapiRef, err := b.AddDirective(
		bucket.NewBuildBucketAPI(bucketID, volumeID),
		bus.NewCallbackHandler(
			func(av directive.AttachedValue) {
				vals <- av.GetValue().(bucket.BuildBucketAPIValue)
			}, nil, nil,
		),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer bapiRef.Release()

	// Expect first value
	emptyVal := <-vals
	if emptyVal.GetExists() || emptyVal.GetBucketConfig() != nil {
		t.Fail()
	}
	t.Log("received first value with exists=false as expected")

	// Apply bucket config
	ap, _, bcRef, err := bus.ExecOneOff(
		ctx,
		b,
		bucket.NewApplyBucketConfigToVolume(
			&bucket.Config{
				Id:  bucketID,
				Rev: 1,
			},
			volumeID,
		),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	bcRef.Release()
	if !ap.GetValue().(bucket.ApplyBucketConfigValue).GetUpdated() {
		t.Fail()
	}

	// Expect second value
	secondVal := <-vals
	if !secondVal.GetExists() || secondVal.GetBucketConfig() == nil {
		t.Fail()
	}
	t.Log("received second value with exists=true as expected")
}
