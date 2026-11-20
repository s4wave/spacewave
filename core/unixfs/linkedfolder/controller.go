// Package core_unixfs_linkedfolder runs linked folders: each keeps one Drive
// equal to a folder on this Device, with the files' blocks read from the
// files.
package core_unixfs_linkedfolder

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	unixfs_linkedfolder "github.com/s4wave/spacewave/sdk/unixfs/linkedfolder"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "unixfs/linked-folder"

// FileSegmentEngine is a World engine whose Device serves blocks from linked
// files: the Space World engine of a Device whose store links files.
type FileSegmentEngine interface {
	// GetFileSegments returns the store serving the blocks of linked files,
	// or nil when the Device's store cannot link files.
	GetFileSegments() *block_store_filesegment.Store
}

// Controller keeps one Drive equal to a folder on this Device. On start it
// syncs the folder with a stat walk, linking only what changed since the
// Drive last matched it.
type Controller struct {
	// le is the logger.
	le *logrus.Entry
	// conf is the config.
	conf *unixfs_linkedfolder.Config
	// engine is the World engine holding the Drive.
	engine *world.BusEngine
}

// NewController constructs a linked-folder controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *unixfs_linkedfolder.Config) *Controller {
	return &Controller{
		le:     le,
		conf:   conf,
		engine: world.NewBusEngine(nil, b, conf.GetEngineId()),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "linked folder controller")
}

// Execute syncs the folder into the Drive, then holds until the context ends.
// A change of the engine serving the World syncs again.
func (c *Controller) Execute(ctx context.Context) error {
	c.engine.SetContext(ctx)
	return world.AccessEngine(ctx, c.engine, func(ctx context.Context, eng world.Engine) error {
		// Require a store that serves linked files.
		fe, ok := eng.(FileSegmentEngine)
		if !ok {
			return errors.New("the World cannot link files")
		}
		files := fe.GetFileSegments()
		if files == nil {
			return errors.New("this Device's store cannot link files")
		}

		// Sync the folder, then hold the engine.
		link, err := unixfs_world.NewFolderLink(eng, files.GetIndex(), c.conf.GetObjectKey(), c.conf.GetFolder(), c.conf.DestPath())
		if err != nil {
			return err
		}
		err = link.Sync(ctx, func(files, bytes uint64) error {
			c.le.WithField("files", files).WithField("bytes", bytes).Debug("linked files")
			return nil
		})
		if err != nil {
			return err
		}
		c.le.Debug("folder synced")
		<-ctx.Done()
		return context.Cause(ctx)
	})
}

// HandleDirective resolves no directives.
func (c *Controller) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
