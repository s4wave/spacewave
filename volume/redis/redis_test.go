//go:build redis_test
// +build redis_test

package volume_redis

import (
	"context"
	"testing"

	volume_test "github.com/aperturerobotics/hydra/volume/test"
	"github.com/sirupsen/logrus"
)

// RedisURL can be overridden from ldflags
var RedisURL = "redis://localhost/"

// TestRedis runs the basic volume test suite against localhost.
func TestRedis(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	vol, err := NewRedis(ctx, le, &Config{
		Url:     RedisURL,
		Verbose: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := volume_test.CheckVolume(ctx, vol); err != nil {
		t.Fatal(err.Error())
	}
}
