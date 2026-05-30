package s4wave_device

import (
	"context"
	"strings"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// DeviceResource implements the DeviceResourceService SRPC interface.
type DeviceResource struct {
	ws     world.WorldState
	engine world.Engine
	objKey string
	state  *Device
	bcast  broadcast.Broadcast
	mux    srpc.Mux
}

// NewDeviceResource creates a new DeviceResource.
func NewDeviceResource(ws world.WorldState, engine world.Engine, objKey string, state *Device) *DeviceResource {
	if state == nil {
		state = &Device{}
	}
	r := &DeviceResource{
		ws:     ws,
		engine: engine,
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
	ctx := strm.Context()

	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	var lastSent *Device
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		state, err := readDeviceObject(ctx, objState)
		if err != nil {
			return err
		}
		if state == nil {
			state = &Device{}
		}

		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.state = state.CloneVT()
			broadcast()
		})

		if lastSent == nil || !state.EqualVT(lastSent) {
			if serr := strm.Send(&WatchDeviceStateResponse{State: state.CloneVT()}); serr != nil {
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

// ReportDeviceStatus updates daemon-owned Device status fields.
func (r *DeviceResource) ReportDeviceStatus(ctx context.Context, req *ReportDeviceStatusRequest) (*ReportDeviceStatusResponse, error) {
	var current *Device
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = r.state.CloneVT()
	})
	if current == nil {
		current = &Device{}
	}

	reqPeerID := strings.TrimSpace(req.GetPeerId())
	if reqPeerID == "" {
		return nil, errors.New("device peer_id is required")
	}
	if reqPeerID != current.GetPeerId() {
		return nil, errors.New("device status peer_id does not match object peer_id")
	}

	updated := current.CloneVT()
	if req.GetSetupState() != DeviceSetupState_DEVICE_SETUP_STATE_UNKNOWN {
		updated.SetupState = req.GetSetupState()
	}
	if req.GetUpdateState() != DeviceUpdateState_DEVICE_UPDATE_STATE_UNKNOWN {
		updated.UpdateState = req.GetUpdateState()
	}
	if req.GetLastStatus() != nil {
		updated.LastStatus = req.GetLastStatus().CloneVT()
	}
	if req.GetReplaceCapabilities() {
		updated.Capabilities = normalizeCapabilities(req.GetCapabilities())
	}
	updated.UpdatedAt = timestamppb.New(time.Now())

	if err := updated.Validate(); err != nil {
		return nil, err
	}
	if err := r.persistState(ctx, updated); err != nil {
		return nil, errors.Wrap(err, "persist device state")
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.state = updated.CloneVT()
		broadcast()
	})

	return &ReportDeviceStatusResponse{State: updated.CloneVT()}, nil
}

func (r *DeviceResource) persistState(ctx context.Context, state *Device) error {
	if r.engine == nil {
		return errors.New("device resource requires a world engine for status updates")
	}
	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	writeState, found, err := wtx.GetObject(ctx, r.objKey)
	if err != nil {
		wtx.Discard()
		return err
	}
	if !found {
		wtx.Discard()
		return world.ErrObjectNotFound
	}
	_, _, err = world.AccessObjectState(ctx, writeState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		wtx.Discard()
		return err
	}
	return wtx.Commit(ctx)
}

func readDeviceObject(ctx context.Context, objState world.ObjectState) (*Device, error) {
	var state *Device
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = UnmarshalDevice(ctx, bcs)
		return uerr
	})
	return state, err
}

func normalizeCapabilities(caps []*DeviceCapability) []*DeviceCapability {
	if len(caps) == 0 {
		return nil
	}
	out := make([]*DeviceCapability, 0, len(caps))
	for _, cap := range caps {
		if cap == nil {
			continue
		}
		out = append(out, normalizeCapability(cap))
	}
	return out
}

func normalizeCapability(cap *DeviceCapability) *DeviceCapability {
	out := cap.CloneVT()
	policy := out.GetPolicy()
	switch {
	case policy.GetLocalState() == DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_DISABLED:
		out.State = DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DISABLED
		if out.Detail == "" {
			out.Detail = "disabled by local policy"
		}
	case policy.GetGrantState() == DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED:
		out.State = DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED
		if out.Detail == "" {
			out.Detail = "blocked by Space grant"
		}
	case out.GetState() == DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE:
		out.State = DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE
	case policy.GetLocalState() == DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED &&
		policy.GetGrantState() == DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED:
		out.State = DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE
	case out.GetState() == DeviceCapabilityState_DEVICE_CAPABILITY_STATE_UNKNOWN:
		out.State = DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED
	}
	return out
}

// _ is a type assertion
var _ SRPCDeviceResourceServiceServer = (*DeviceResource)(nil)
