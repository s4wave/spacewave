package s4wave_device

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// DeviceResource implements the DeviceResourceService SRPC interface.
type DeviceResource struct {
	// bus resolves the observing Session's authenticated transport.
	bus bus.Bus
	// sessionPeerID is the observing Session selected by the granting mount.
	sessionPeerID peer.ID
	// le records resource failures.
	le *logrus.Entry
	// engine supplies checkout-root transactions.
	engine world.Engine
	// ws is the granted World state.
	ws world.WorldState
	// objKey identifies this Device in the World.
	objKey string
	// mux dispatches Device resource methods.
	mux srpc.Mux
}

// NewDeviceResource creates a new DeviceResource.
func NewDeviceResource(le *logrus.Entry, b bus.Bus, sessionPeerID peer.ID, engine world.Engine, ws world.WorldState, objKey string) *DeviceResource {
	// Retain the World grant and observing Session for Device methods.
	r := &DeviceResource{
		bus:           b,
		sessionPeerID: sessionPeerID,
		le:            le,
		engine:        engine,
		ws:            ws,
		objKey:        objKey,
	}

	// Register the Device service on its resource mux.
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterDeviceResourceService(mux, r)
	})
	return r
}

// WatchDeviceAvailability streams the Device's durable record and live reachability.
func (r *DeviceResource) WatchDeviceAvailability(_ *WatchDeviceAvailabilityRequest, strm SRPCDeviceResourceService_WatchDeviceAvailabilityStream) error {
	return WatchDeviceAvailability(strm.Context(), r.bus, r.sessionPeerID, r.ws, r.objKey, strm.Send)
}

// GetMux returns the srpc mux for this resource.
func (r *DeviceResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchDeviceState streams Device state changes from world object revisions.
func (r *DeviceResource) WatchDeviceState(_ *WatchDeviceStateRequest, strm SRPCDeviceResourceService_WatchDeviceStateStream) error {
	return s4wave_world.WatchWorldObject(strm.Context(), r.ws, r.objKey, readDeviceObject,
		func(state *Device, changed bool) error {
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

// AccessCheckoutRoot publishes a filesystem handle for a World object or a live Device folder.
func (r *DeviceResource) AccessCheckoutRoot(ctx context.Context, req *AccessCheckoutRootRequest) (*AccessCheckoutRootResponse, error) {
	// Resolve the caller that owns the returned resource.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// For a write, select from the latest Device record when an Engine is available.
	write := req.GetWrite()
	writeApprovalRef := strings.TrimSpace(req.GetWriteApprovalRef())
	if write && writeApprovalRef == "" {
		return nil, errors.New("checkout root write access requires approval ref")
	}
	stateWS := r.ws
	if write && r.engine != nil {
		stateWS = world.NewEngineWorldState(r.engine, true)
	}
	state, err := world.LookupObjectBody[Device](ctx, stateWS, r.objKey)
	if err != nil {
		return nil, err
	}
	if !state.IsSelectable() {
		return nil, errors.New("device is not selectable")
	}

	// Apply the Device's published selection and access mode before opening the link.
	var capability *DeviceCapability
	if write {
		capability = state.FindWritableCheckoutRoot(req.GetName())
	} else {
		capability = state.FindReadableCheckoutRoot(req.GetName())
	}
	if capability == nil {
		if write {
			return nil, errors.New("checkout root is not writable")
		}
		return nil, errors.New("checkout root is not readable")
	}

	// Open the capability on the Device's Session or in the granted World.
	capLink := capability.GetLink()
	protocolID := protocol.ID(strings.TrimSpace(capLink.GetProtocolId()))
	var resourceID uint32
	switch {
	case protocolID != "":
		resourceID, err = r.openLiveCheckoutRoot(ctx, resourceCtx, state, protocolID, write)
	case strings.TrimSpace(capLink.GetTypeId()) == s4wave_unixfs_world.UnixFSTypeID:
		resourceID, err = r.openWorldCheckoutRoot(ctx, resourceCtx, stateWS, capLink.GetObjectKey(), write)
	default:
		return nil, errors.Errorf("checkout root %q is not a unixfs owner", capability.GetId())
	}
	if err != nil {
		return nil, err
	}

	// Return the selected capability and the resource both link types share.
	return &AccessCheckoutRootResponse{
		ResourceId:       resourceID,
		CapabilityId:     capability.GetId(),
		ObjectKey:        capLink.GetObjectKey(),
		TypeId:           capLink.GetTypeId(),
		CheckoutRoot:     capability.GetCheckoutRoot().CloneVT(),
		WriteAvailable:   DeviceCapabilityCanWriteCheckoutRoot(capability),
		WriteEnabled:     write,
		WriteApprovalRef: writeApprovalRef,
	}, nil
}

// openWorldCheckoutRoot publishes a cursor attached to the linked World object.
func (r *DeviceResource) openWorldCheckoutRoot(ctx context.Context, resourceCtx resource_server.ResourceClientContext, stateWS world.WorldState, key string, write bool) (uint32, error) {
	// Open the World object with the requested access, preserving the World's write grant.
	accessWS := stateWS
	if write && accessWS.GetReadOnly() {
		return 0, errors.New("write checkout root access requires an engine-backed Device resource")
	}
	if !write {
		var err error
		accessWS, err = r.readOnlyWorldState()
		if err != nil {
			return 0, err
		}
	}
	fsType, _, err := unixfs_world.LookupFsType(ctx, accessWS, key)
	if err != nil {
		return 0, err
	}

	// Use the World cursor so the handle retains object-backed editing and watches.
	var cursor *unixfs_world.FSCursor
	if write {
		cursor, _ = unixfs_world.NewFSCursorWithWriter(ctx, r.le, accessWS, key, fsType, "")
	} else {
		cursor = unixfs_world.NewFSCursor(r.le, accessWS, key, fsType, nil, true)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		return 0, err
	}

	// Transfer the filesystem handle to the caller's resource tree.
	res := resource_unixfs.NewFSHandleObjectResource(r.le, handle, cursor, nil, accessWS, key, fsType, nil)
	id, err := resourceCtx.AddResourceValue(res.GetMux(), res, handle.Release)
	if err != nil {
		handle.Release()
	}
	return id, err
}

// readOnlyWorldState returns the granted World with writing disabled.
func (r *DeviceResource) readOnlyWorldState() (world.WorldState, error) {
	if r.engine != nil {
		return world.NewEngineWorldState(r.engine, false), nil
	}
	if r.ws.GetReadOnly() {
		return r.ws, nil
	}
	return nil, errors.New("read-only checkout root access requires an engine-backed Device resource")
}

// readDeviceObject decodes the Device state held at the object's root.
func readDeviceObject(ctx context.Context, objState world.ObjectState) (*Device, error) {
	// Decode the Device block; an empty object has the initial Device state.
	state, err := s4wave_world.ReadWorldBlock[Device](ctx, objState)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &Device{}
	}
	return state, nil
}

// _ is a type assertion.
var _ SRPCDeviceResourceServiceServer = (*DeviceResource)(nil)
