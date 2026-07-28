//go:build redis_test

package volume_redis

import (
	"context"
	"errors"
	"testing"

	store_kvtx_redis "github.com/s4wave/spacewave/db/store/kvtx/redis"
	"github.com/s4wave/spacewave/db/volume"
	volume_test "github.com/s4wave/spacewave/db/volume/test"
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
		Client:  &store_kvtx_redis.ClientConfig{Url: RedisURL},
		Verbose: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := volume_test.CheckVolume(ctx, vol); err != nil {
		t.Fatal(err.Error())
	}
}
func TestRedisWorldEngineLease(t *testing.T) {
	ctx := context.Background()
	vol, err := NewRedis(ctx, logrus.NewEntry(logrus.New()), &Config{
		Client: &store_kvtx_redis.ClientConfig{Url: RedisURL},
	})
	if err != nil {
		t.Fatal(err)
	}

	lease, err := vol.AcquireWorldEngineLease(ctx, "shared-object")
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	t.Cleanup(func() { _ = lease.Release() })

	_, err = vol.AcquireWorldEngineLease(ctx, "shared-object")
	var heldErr *volume.WorldEngineLeaseHeldError
	if !errors.As(err, &heldErr) {
		t.Fatalf("second lease error = %v, want WorldEngineLeaseHeldError", err)
	}
}
