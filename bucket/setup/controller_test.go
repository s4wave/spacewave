package bucket_setup

import (
	"context"
	"regexp"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	bucket_mock "github.com/aperturerobotics/hydra/bucket/mock"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestSetupController tests the setup controller.
func TestSetupController(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testbed.Verbose = true
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	b := tb.Bus
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	bucketID := "setup-this-bucket"
	conf := &Config{
		ApplyBucketConfigs: []*ApplyBucketConfig{{
			Config:     bucket_mock.NewMockBucketConfig(bucketID, 1),
			VolumeIdRe: regexp.QuoteMeta(volID),
		}},
	}

	f := NewFactory(b)
	ctrl, err := f.Construct(conf, controller.ConstructOpts{
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
	info, err := vol.GetBucketInfo(bucketID)
	if err != nil {
		t.Fatal(err.Error())
	}
	if info.GetConfig().GetId() != bucketID {
		t.FailNow()
	}

	t.Log("successfully configured bucket")
}
