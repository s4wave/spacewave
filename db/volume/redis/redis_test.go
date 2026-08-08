//go:build redis_test

package volume_redis

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/conformance"
	store_kvtx_redis "github.com/s4wave/spacewave/db/store/kvtx/redis"
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

func TestRedisKeyedWriteLease(t *testing.T) {
	ctx := context.Background()
	vol, err := NewRedis(ctx, logrus.NewEntry(logrus.New()), &Config{
		Client: &store_kvtx_redis.ClientConfig{Url: RedisURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := coord.Scope{
		VolumeID:      vol.GetID(),
		ParticipantID: "first",
		Key:           "shared-object",
	}

	lease, ok, err := vol.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("acquire first lease: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { _ = lease.Release(ctx) })

	if _, ok, err := vol.TryAcquireWriteLease(ctx, scope); err != nil || ok {
		t.Fatalf("second lease = ok=%v err=%v, want held", ok, err)
	}
}

func TestRedisCoordinatorConformance(t *testing.T) {
	ctx := context.Background()
	conformance.Check(t, func(tb testing.TB) (coord.Coordinator, coord.Coordinator) {
		newVolume := func() *Redis {
			vol, err := NewRedis(ctx, logrus.NewEntry(logrus.New()), &Config{
				Client: &store_kvtx_redis.ClientConfig{Url: RedisURL},
			})
			if err != nil {
				tb.Fatal(err)
			}
			tb.Cleanup(func() { _ = vol.Close() })
			return vol
		}
		return newVolume(), newVolume()
	})
}
