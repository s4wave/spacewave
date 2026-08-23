package s4wave_device

import (
	"context"
	"strings"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// DeviceResource implements the DeviceResourceService SRPC interface.
type DeviceResource struct {
	le     *logrus.Entry
	engine world.Engine
	ws     world.WorldState
	objKey string
	state  *Device
	bcast  broadcast.Broadcast
	mux    srpc.Mux
}

// NewDeviceResource creates a new DeviceResource.
func NewDeviceResource(le *logrus.Entry, engine world.Engine, ws world.WorldState, objKey string, state *Device) *DeviceResource {
	if state == nil {
		state = &Device{}
	}
	r := &DeviceResource{
		le:     le,
		engine: engine,
		ws:     ws,
		objKey: objKey,
		state:  state,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterDeviceResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *DeviceResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchDeviceState streams Device state changes from world object revisions.
func (r *DeviceResource) WatchDeviceState(_ *WatchDeviceStateRequest, strm SRPCDeviceResourceService_WatchDeviceStateStream) error {
	return s4wave_world.WatchWorldObject(strm.Context(), r.ws, r.objKey, readDeviceObject,
		func(state *Device, changed bool) error {
			r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				r.state = state.CloneVT()
				broadcast()
			})
			if changed {
				return strm.Send(&WatchDeviceStateResponse{State: state.CloneVT()})
			}
			return nil
		})
}

// ReportDeviceStatus rejects browser-visible status writes.
func (r *DeviceResource) ReportDeviceStatus(ctx context.Context, req *ReportDeviceStatusRequest) (*ReportDeviceStatusResponse, error) {
	return nil, errors.New("device status reports are daemon-owned and unavailable through typed object resources")
}

// AccessCheckoutRoot opens the selected checkout root's filesystem object as an
// FSHandle resource. Write access requires explicit capability and approval
// metadata before Device creates a writer-backed UnixFS cursor.
func (r *DeviceResource) AccessCheckoutRoot(ctx context.Context, req *AccessCheckoutRootRequest) (*AccessCheckoutRootResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	writeApprovalRef := strings.TrimSpace(req.GetWriteApprovalRef())
	var accessWS world.WorldState
	if req.GetWrite() {
		if writeApprovalRef == "" {
			return nil, errors.New("checkout root write access requires approval ref")
		}
		accessWS, err = r.writeWorldState()
	} else {
		accessWS, err = r.readOnlyWorldState()
	}
	if err != nil {
		return nil, err
	}

	stateWS := r.ws
	if req.GetWrite() {
		stateWS = accessWS
	}
	objState, found, err := stateWS.GetObject(ctx, r.objKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}

	state, err := readDeviceObject(ctx, objState)
	if err != nil {
		return nil, err
	}
	if !state.IsSelectable() {
		return nil, errors.New("device is not selectable")
	}

	var capability *DeviceCapability
	if req.GetWrite() {
		capability = state.FindWritableCheckoutRoot(req.GetName())
	} else {
		capability = state.FindReadableCheckoutRoot(req.GetName())
	}
	if capability == nil {
		if req.GetWrite() {
			return nil, errors.New("checkout root is not writable")
		}
		return nil, errors.New("checkout root is not readable")
	}
	link := capability.GetLink()
	if strings.TrimSpace(link.GetTypeId()) != s4wave_unixfs_world.UnixFSTypeID {
		return nil, errors.Errorf("checkout root %q is not a unixfs owner", capability.GetId())
	}

	fsType, _, err := unixfs_world.LookupFsType(ctx, accessWS, link.GetObjectKey())
	if err != nil {
		return nil, err
	}
	var fsCursor *unixfs_world.FSCursor
	if req.GetWrite() {
		fsCursor, _ = unixfs_world.NewFSCursorWithWriter(ctx, r.le, accessWS, link.GetObjectKey(), fsType, "")
	} else {
		fsCursor = unixfs_world.NewFSCursor(r.le, accessWS, link.GetObjectKey(), fsType, nil, true)
	}
	fsh, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, err
	}

	fsResource := resource_unixfs.NewFSHandleObjectResource(
		r.le,
		fsh,
		nil,
		accessWS,
		link.GetObjectKey(),
		fsType,
		nil,
	)
	resourceID, err := resourceCtx.AddResourceValue(fsResource.GetMux(), fsResource, func() {
		fsh.Release()
	})
	if err != nil {
		fsh.Release()
		return nil, err
	}

	return &AccessCheckoutRootResponse{
		ResourceId:       resourceID,
		CapabilityId:     capability.GetId(),
		ObjectKey:        link.GetObjectKey(),
		TypeId:           link.GetTypeId(),
		CheckoutRoot:     capability.GetCheckoutRoot().CloneVT(),
		WriteAvailable:   DeviceCapabilityCanWriteCheckoutRoot(capability),
		WriteEnabled:     req.GetWrite(),
		WriteApprovalRef: writeApprovalRef,
	}, nil
}

func (r *DeviceResource) readOnlyWorldState() (world.WorldState, error) {
	if r.engine != nil {
		return world.NewEngineWorldState(r.engine, false), nil
	}
	if r.ws.GetReadOnly() {
		return r.ws, nil
	}
	return nil, errors.New("read-only checkout root access requires an engine-backed Device resource")
}

func (r *DeviceResource) writeWorldState() (world.WorldState, error) {
	if r.engine != nil {
		return world.NewEngineWorldState(r.engine, true), nil
	}
	if !r.ws.GetReadOnly() {
		return r.ws, nil
	}
	return nil, errors.New("write checkout root access requires an engine-backed Device resource")
}

func readDeviceObject(ctx context.Context, objState world.ObjectState) (*Device, error) {
	state, err := s4wave_world.ReadWorldBlock[*Device](ctx, objState, NewDeviceBlock)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &Device{}
	}
	return state, nil
}

var _ SRPCDeviceResourceServiceServer = (*DeviceResource)(nil)
