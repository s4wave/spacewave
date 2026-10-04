package bucket_setup

import (
	"context"
	"regexp"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	bucket_mock "github.com/s4wave/spacewave/db/bucket/mock"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestSetupController tests the setup controller.
func TestSetupController(t *testing.T) {
	// Prepare logging and context for the bucket setup scenario.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a verbose testbed and register the bucket setup factory.
	testbed.Verbose = true
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Register the setup controller factory with the testbed resolver.
	tb.StaticResolver.AddFactory(bucket_setup.NewFactory(tb.Bus))

	// Read the testbed bus and Volume needed by the setup config.
	b := tb.Bus
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// Configure the bucket the setup controller must create.
	bucketID := "setup-this-bucket"
	conf := &bucket_setup.Config{
		ApplyBucketConfigs: []*bucket_setup.ApplyBucketConfig{{
			Config:     bucket_mock.NewMockBucketConfig(bucketID, 1),
			VolumeIdRe: regexp.QuoteMeta(volID),
		}},
	}

	// Construct and execute the setup controller against the test bus.
	f := bucket_setup.NewFactory(b)
	ctrl, err := f.Construct(ctx, conf, controller.ConstructOpts{
		Logger: le,
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// expect exit after applying
	err = b.ExecuteController(ctx, ctrl)
	if err != nil {
		t.Fatal(err.Error())
	}

	// close
	err = ctrl.Close()
	if err != nil {
		t.Fatal(err.Error())
	}

	// check if config applied
	info, err := vol.GetBucketInfo(ctx, bucketID)
	if err != nil {
		t.Fatal(err.Error())
	}
	if info.GetConfig().GetId() != bucketID {
		t.FailNow()
	}

	// Record successful bucket configuration after verifying its stored info.
	t.Log("successfully configured bucket")
}
