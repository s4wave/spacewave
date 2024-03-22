package dist_entrypoint

import (
	"context"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/go-kvfile"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	volume_kvfile "github.com/aperturerobotics/hydra/volume/kvfile"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// StaticVolumeController manages the static kvfile volume.
type StaticVolumeController struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// file is the volume.kvfile
	file     io.ReaderAt
	fileSize uint64
	// volConf is the volume config
	volConf *volume_kvfile.Config
	// close is the close callback
	close func()
}

// NewStaticVolumeController constructs a new static volume controller.
func NewStaticVolumeController(
	le *logrus.Entry,
	b bus.Bus,
	f io.ReaderAt,
	fileSize uint64,
	volConf *volume_kvfile.Config,
	close func(),
) *StaticVolumeController {
	if volConf == nil {
		volConf = &volume_kvfile.Config{}
	}
	if volConf.VolumeConfig == nil {
		volConf.VolumeConfig = &volume_controller.Config{}
	}

	// security: disable loading peer from the volume
	volConf.VolumeConfig.DisablePeer = true

	// performance: disable reconciler queues
	volConf.VolumeConfig.DisableEventBlockRm = true
	volConf.VolumeConfig.DisableReconcilerQueues = true

	return &StaticVolumeController{le: le, b: b, file: f, fileSize: fileSize, close: close, volConf: volConf}
}

// GetControllerInfo returns information about the controller.
func (c *StaticVolumeController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("entrypoint/static-volume", semver.MustParse("0.0.1"), "entrypoint static volume")
}

// HandleDirective asks if the handler can resolve the directive.
func (c *StaticVolumeController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Execute executes the controller goroutine.
func (c *StaticVolumeController) Execute(ctx context.Context) error {
	reader, err := kvfile.BuildReader(c.file, c.fileSize)
	if err != nil {
		return err
	}

	vc, err := volume_kvfile.NewVolumeController(ctx, c.le, c.b, c.volConf, reader)
	if err != nil {
		return err
	}

	return c.b.ExecuteController(ctx, vc)
}

// Close releases any resources used by the controller.
func (c *StaticVolumeController) Close() error {
	if c.close != nil {
		c.close()
	}
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*StaticVolumeController)(nil))
