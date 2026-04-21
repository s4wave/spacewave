package volume_redis

import (
	"context"

	"github.com/blang/semver/v4"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	skvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the Redis volume controller.
const ControllerID = "hydra/volume/redis"

// Version is the version of the redis implementation.
var Version = semver.MustParse("0.0.1")

// Redis implements a RedisDB backed volume.
type Redis = kvtx.Volume

// NewRedis builds a new Redis volume, opening the database.
func NewRedis(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*Redis, error) {
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	redisOpts, err := conf.BuildRedisOptions()
	if err != nil {
		return nil, err
	}

	store, err := conf.GetClient().Connect(
		ctx,
		redisOpts...,
	)
	if err != nil {
		return nil, err
	}
	store.SetContext(ctx)

	var vstore skvtx.Store = store
	if conf.GetVerbose() {
		vstore = kvtx_vlogger.NewVLogger(le, vstore)
	}

	return kvtx.NewVolume(
		ctx,
		ControllerID,
		kvkey,
		vstore,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		nil,
		store.GetPool().Close,
	)
}
