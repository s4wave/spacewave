package s4wave_drive

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// DriveTypeID is the object type ID for Drive objects.
const DriveTypeID = "spacewave/drive"

// DriveObjectKey is the default Drive object key used by quickstart.
const DriveObjectKey = "drive"

// DriveResource implements the DriveResourceService SRPC interface.
type DriveResource struct {
	ws     world.WorldState
	objKey string
	mux    srpc.Mux
}

// NewDriveResource creates a new DriveResource.
func NewDriveResource(ws world.WorldState, objKey string) *DriveResource {
	r := &DriveResource{
		ws:     ws,
		objKey: objKey,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterDriveResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *DriveResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchDriveState streams Drive state changes.
func (r *DriveResource) WatchDriveState(_ *WatchDriveStateRequest, strm SRPCDriveResourceService_WatchDriveStateStream) error {
	ctx := strm.Context()

	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	var lastSent *Drive
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		var state *Drive
		_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
			var err error
			state, err = UnmarshalDrive(ctx, bcs)
			return err
		})
		if err != nil {
			return err
		}
		if state == nil {
			state = &Drive{}
		}

		if lastSent == nil || !state.EqualVT(lastSent) {
			if serr := strm.Send(&WatchDriveStateResponse{State: state.CloneVT()}); serr != nil {
				return serr
			}
			lastSent = state
		}

		_, err = objState.WaitRev(ctx, rev+1, false)
		if err != nil {
			return err
		}
	}
}

// NewDriveBlock constructs a new Drive block.
func NewDriveBlock() block.Block {
	return &Drive{}
}

// UnmarshalDrive unmarshals Drive state from a cursor.
func UnmarshalDrive(ctx context.Context, bcs *block.Cursor) (*Drive, error) {
	return block.UnmarshalBlock[*Drive](ctx, bcs, NewDriveBlock)
}

// MarshalBlock marshals the Drive to bytes.
func (d *Drive) MarshalBlock() ([]byte, error) {
	return d.MarshalVT()
}

// UnmarshalBlock unmarshals the Drive from bytes.
func (d *Drive) UnmarshalBlock(data []byte) error {
	return d.UnmarshalVT(data)
}

// Validate performs cursory checks on the Drive state.
func (d *Drive) Validate() error {
	return nil
}

// _ is a type assertion
var _ SRPCDriveResourceServiceServer = (*DriveResource)(nil)

// _ is a type assertion
var _ block.Block = (*Drive)(nil)
