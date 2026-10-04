package volume_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// This file contains some limited tests.
// Other volume-specific e2e tests are done elsewhere.

// TestBucketHandleFlush tests looking for a bucket, not finding it, then
// pushing it with a ApplyBucketConfig: we then expect the controller to
// re-check for the configuration, find it, and create new handles.
func TestBucketHandleFlush(t *testing.T) {
	// Prepare the context and logger used by the testbed.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the testbed that owns the controller bus and volume.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Identify the bucket and volume used by the directives.
	bucketID := "test-bucket-flush"
	b := tb.Bus
	vol := tb.Volume
	volumeID := vol.GetID()

	// Request a bucket API before its configuration exists.
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

	// Apply the configuration that should make the bucket available.
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

	// Verify that the bucket API reports the applied configuration.
	secondVal := <-vals
	if !secondVal.GetExists() || secondVal.GetBucketConfig() == nil {
		t.Fail()
	}
	t.Log("received second value with exists=true as expected")
}
