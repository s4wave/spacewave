package volume_redis

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	skvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the Redis volume controller.
const ControllerID = "hydra/volume/redis"

// Version is the version of the redis implementation.
var Version = controller.MustParseVersion("0.0.1")

// Redis implements a RedisDB backed volume.
type Redis = kvtx.Volume

// NewRedis builds a new Redis volume, opening the database.
func NewRedis(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*Redis, error) {
	// Build the key codec from the configuration.
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	// Build the redis connection options.
	redisOpts, err := conf.BuildRedisOptions()
	if err != nil {
		return nil, err
	}

	// Connect the redis client with this controller's client name.
	store, err := conf.GetClient().ConnectWithClientName(
		ctx,
		redisClientName,
		redisOpts...,
	)
	if err != nil {
		return nil, err
	}
	store.SetContext(ctx)

	// Wrap the store in a logger when verbose logging is enabled.
	var vstore skvtx.Store = store
	if conf.GetVerbose() {
		vstore = kvtx_vlogger.NewVLogger(le, vstore)
	}

	// Construct the volume over the store.
	vol, err := kvtx.NewVolume(
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
	if err != nil {
		return nil, err
	}
	vol.Coordinator = NewCoordinator(
		store.GetPool(),
		string(kvkey.GetObjectStorePrefixByID(ControllerID)),
		vol.Coordinator,
	)
	return vol, nil
}
