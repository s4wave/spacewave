package block_store_http_server

import (
	"context"
	"net/http"

	bifrost_http "github.com/aperturerobotics/bifrost/http"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/blang/semver"
)

// ControllerID is the controller identifier.
const ControllerID = "hydra/block/store/http/server"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description
var controllerDescrip = "serves http block store"

// Controller is the block store http server controller.
//
// Handles LookupHTTPHandler with the block store endpoints.
type Controller = bifrost_http.HTTPHandlerController

// NewController constructs a new http handler controller.
func NewController(b bus.Bus, conf *Config) *Controller {
	return bifrost_http.NewHTTPHandlerController(
		controller.NewInfo(
			ControllerID,
			Version,
			controllerDescrip,
		),
		func(ctx context.Context, released func()) (*http.Handler, func(), error) {
			// Lookup the bucket.
			bkt, bktRel, err := bucket_lookup.StartBucketRWOperation(ctx, b, &bucket.BucketOpArgs{
				BucketId: conf.GetBucketId(),
				VolumeId: conf.GetVolumeId(),
			})
			if err != nil {
				return nil, nil, err
			}

			srv := NewHTTPBlock(bkt, conf.GetWrite(), conf.GetPathPrefix(), conf.GetForceHashType())
			var handler http.Handler = srv
			return &handler, bktRel, nil
		},
		[]string{conf.GetPathPrefix()},
		false,
		nil,
	)
}
