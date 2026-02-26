package block_store_kvfile_http

import (
	"context"
	"net/textproto"

	"github.com/aperturerobotics/controllerbus/controller"
	block_store "github.com/aperturerobotics/hydra/block/store"
	block_store_controller "github.com/aperturerobotics/hydra/block/store/controller"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the kvfile http block store controller.
const ControllerID = "hydra/block/store/kvfile/http"

// Version is the version of the block store implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the kvfile http block store controller.
type Controller = block_store_controller.Controller

// NewController builds a new kvfile http block store controller.
func NewController(le *logrus.Entry, conf *Config) *Controller {
	return block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID, Version, "kvfile via http block store"),
		NewBlockStoreBuilder(le, conf, conf.GetVerbose()),
		[]string{conf.GetBlockStoreId()},
		true,
		conf.GetBucketIds(),
		conf.GetSkipNotFound(),
		conf.GetVerbose(),
	)
}

// NewBlockStoreBuilder constructs a new block store builder from config.
//
// le can be nil to disable logging
// verbose logs successes as well as failures
func NewBlockStoreBuilder(le *logrus.Entry, conf *Config, verbose bool) block_store_controller.BlockStoreBuilder {
	return func(ctx context.Context, released func()) (block_store.Store, func(), error) {
		fileURL, err := conf.ParseURL()
		if err != nil {
			return nil, nil, err
		}

		kvk, err := store_kvkey.NewKVKey(conf.GetKvKeyOpts())
		if err != nil {
			return nil, nil, err
		}

		var headers map[string][]string
		if cheaders := conf.GetHeaders(); len(cheaders) != 0 {
			headers = make(map[string][]string, len(cheaders))
			for key, value := range cheaders {
				headers[textproto.CanonicalMIMEHeaderKey(key)] = []string{value}
			}
		}

		kvfileBlock, err := NewKvfileHTTPBlock(
			ctx,
			le,
			fileURL.String(),
			headers,
			conf.GetDisableCache(),
			kvk,
			int64(conf.GetMinRequestSize()), //nolint:gosec
			verbose,
		)
		if err != nil {
			return nil, nil, err
		}

		blockStore := block_store.NewStore(conf.GetBlockStoreId(), kvfileBlock)

		/*
			if verbose {
				blockStore = block_store_vlogger.NewVLoggerStore(le, blockStore)
			}
		*/

		return blockStore, nil, nil
	}
}
