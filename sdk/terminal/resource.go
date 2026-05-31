package s4wave_terminal

import (
	"context"
	stderrors "errors"
	"io"
	"slices"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

const terminalFrameMaxBytes = 4 * 1024 * 1024

type terminalConnectResult struct {
	err          error
	updateState  bool
	finalState   TerminalSessionState
	status       string
	errorMessage string
}

// TerminalResource implements the TerminalResourceService SRPC interface.
type TerminalResource struct {
	b      bus.Bus
	ws     world.WorldState
	engine world.Engine
	objKey string
	state  *Terminal
	bcast  broadcast.Broadcast
	mux    srpc.Mux
}

// NewTerminalResource creates a new TerminalResource.
func NewTerminalResource(
	b bus.Bus,
	ws world.WorldState,
	engine world.Engine,
	objKey string,
	state *Terminal,
) *TerminalResource {
	if state == nil {
		state = &Terminal{}
	}
	r := &TerminalResource{
		b:      b,
		ws:     ws,
		engine: engine,
		objKey: objKey,
		state:  state,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterTerminalResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *TerminalResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchTerminalState streams Terminal state changes from world object revisions.
func (r *TerminalResource) WatchTerminalState(_ *WatchTerminalStateRequest, strm SRPCTerminalResourceService_WatchTerminalStateStream) error {
	ctx := strm.Context()

	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	var lastSent *Terminal
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		state, err := readTerminalObject(ctx, objState)
		if err != nil {
			return err
		}
		if state == nil {
			state = &Terminal{}
		}

		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.state = state.CloneVT()
			broadcast()
		})

		if lastSent == nil || !state.EqualVT(lastSent) {
			if serr := strm.Send(&WatchTerminalStateResponse{State: state.CloneVT()}); serr != nil {
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

// ConnectTerminal opens the live stream for this Terminal target.
func (r *TerminalResource) ConnectTerminal(strm SRPCTerminalResourceService_ConnectTerminalStream) error {
	ctx, cancel := context.WithCancel(strm.Context())
	defer cancel()

	current := r.currentState()
	if err := current.Validate(); err != nil {
		return err
	}

	switch EffectiveTerminalTargetKind(current) {
	case TerminalTargetKind_TERMINAL_TARGET_KIND_DEVICE:
		return r.connectDeviceTerminal(ctx, cancel, strm, current)
	case TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST:
		return r.connectSshHostTerminal(ctx, cancel, strm, current)
	default:
		return errors.New("terminal target is required")
	}
}

func (r *TerminalResource) connectDeviceTerminal(
	ctx context.Context,
	cancel context.CancelFunc,
	strm SRPCTerminalResourceService_ConnectTerminalStream,
	current *Terminal,
) error {
	if r.b == nil {
		return errors.New("terminal resource requires a bus to connect")
	}
	remotePeer, err := peer.IDB58Decode(current.GetDevicePeerId())
	if err != nil {
		return errors.Wrap(err, "terminal device peer id")
	}

	if err := r.updateState(ctx, TerminalSessionState_TERMINAL_SESSION_STATE_CONNECTING, "connecting", ""); err != nil {
		return err
	}

	ms, release, err := link.OpenStreamWithPeerEx(
		ctx,
		r.b,
		RemoteShellProtocolID,
		"",
		remotePeer,
		0,
		stream.OpenOpts{},
	)
	if err != nil {
		state, status, errMessage := terminalConnectOpenFailureState(ctx, err, "failed to connect")
		_ = r.updateState(context.Background(), state, status, errMessage)
		return err
	}
	defer release()
	defer ms.GetStream().Close()

	frameSession := stream_packet.NewSession(ms.GetStream(), terminalFrameMaxBytes)
	cols, rows := NormalizeTerminalFrameSize(current.GetCols(), current.GetRows())
	if err := frameSession.SendMsg(&TerminalFrame{
		Kind:        TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN,
		Cols:        cols,
		Rows:        rows,
		Command:     current.GetCommand(),
		Environment: slices.Clone(current.GetEnvironment()),
	}); err != nil {
		state, status, errMessage := terminalConnectOpenFailureState(ctx, err, "failed to open")
		_ = r.updateState(context.Background(), state, status, errMessage)
		return err
	}

	errCh := make(chan terminalConnectResult, 2)
	var clientClosed atomic.Bool
	go r.forwardClientFrames(ctx, strm, frameSession, &clientClosed, errCh)
	go r.forwardRemoteFrames(ctx, strm, frameSession, &clientClosed, errCh)

	result := <-errCh
	cancel()
	if result.err != nil && !stderrors.Is(result.err, context.Canceled) && !stderrors.Is(result.err, io.EOF) {
		_ = r.updateState(context.Background(), TerminalSessionState_TERMINAL_SESSION_STATE_FAILED, "terminal failed", result.err.Error())
		return result.err
	}
	if result.updateState {
		return r.updateState(context.Background(), result.finalState, result.status, result.errorMessage)
	}
	return nil
}

func terminalConnectOpenFailureState(ctx context.Context, err error, status string) (TerminalSessionState, string, string) {
	if ctx.Err() != nil {
		return TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED, "disconnected", ""
	}
	return TerminalSessionState_TERMINAL_SESSION_STATE_FAILED, status, err.Error()
}

func (r *TerminalResource) forwardClientFrames(
	ctx context.Context,
	strm SRPCTerminalResourceService_ConnectTerminalStream,
	frameSession *stream_packet.Session,
	clientClosed *atomic.Bool,
	errCh chan<- terminalConnectResult,
) {
	for {
		frame, err := strm.Recv()
		if err != nil {
			_ = frameSession.SendMsg(&TerminalFrame{Kind: TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE})
			errCh <- terminalConnectResult{
				err:         err,
				updateState: true,
				finalState:  TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED,
				status:      "disconnected",
			}
			return
		}
		if frame == nil {
			continue
		}
		switch frame.GetKind() {
		case TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT,
			TerminalFrameKind_TERMINAL_FRAME_KIND_RESIZE,
			TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE:
			if frame.GetKind() == TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE {
				clientClosed.Store(true)
			}
			if err := frameSession.SendMsg(frame); err != nil {
				errCh <- terminalConnectResult{err: err}
				return
			}
			if frame.GetKind() == TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE {
				return
			}
		default:
			errCh <- terminalConnectResult{err: errors.Errorf("unsupported terminal client frame kind %s", frame.GetKind().String())}
			return
		}
		if err := ctx.Err(); err != nil {
			errCh <- terminalConnectResult{
				err:         err,
				updateState: true,
				finalState:  TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED,
				status:      "disconnected",
			}
			return
		}
	}
}

func (r *TerminalResource) forwardRemoteFrames(
	ctx context.Context,
	strm SRPCTerminalResourceService_ConnectTerminalStream,
	frameSession *stream_packet.Session,
	clientClosed *atomic.Bool,
	errCh chan<- terminalConnectResult,
) {
	for {
		frame := &TerminalFrame{}
		if err := frameSession.RecvMsg(frame); err != nil {
			finalState, status, errMessage := terminalConnectDisconnectState(clientClosed.Load())
			errCh <- terminalConnectResult{
				err:          err,
				updateState:  true,
				finalState:   finalState,
				status:       status,
				errorMessage: errMessage,
			}
			return
		}
		switch frame.GetKind() {
		case TerminalFrameKind_TERMINAL_FRAME_KIND_READY:
			if err := r.updateState(ctx, TerminalSessionState_TERMINAL_SESSION_STATE_ACTIVE, "active", ""); err != nil {
				errCh <- terminalConnectResult{err: err}
				return
			}
		case TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR:
			errMessage := frame.GetError()
			if errMessage == "" {
				errMessage = "terminal failed"
			}
			_ = r.updateState(ctx, TerminalSessionState_TERMINAL_SESSION_STATE_FAILED, "terminal failed", errMessage)
			if err := strm.Send(frame); err != nil {
				errCh <- terminalConnectResult{err: err}
				return
			}
			errCh <- terminalConnectResult{err: errors.New(errMessage)}
			return
		case TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT:
			finalState, status, errMessage := terminalConnectExitState(clientClosed.Load())
			if err := r.updateState(ctx, finalState, status, errMessage); err != nil {
				errCh <- terminalConnectResult{err: err}
				return
			}
		}
		if err := strm.Send(frame); err != nil {
			errCh <- terminalConnectResult{err: err}
			return
		}
		if frame.GetKind() == TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT ||
			frame.GetKind() == TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR {
			errCh <- terminalConnectResult{}
			return
		}
		if err := ctx.Err(); err != nil {
			errCh <- terminalConnectResult{
				err:         err,
				updateState: true,
				finalState:  TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED,
				status:      "disconnected",
			}
			return
		}
	}
}

func terminalConnectDisconnectState(clientClosed bool) (TerminalSessionState, string, string) {
	if clientClosed {
		return TerminalSessionState_TERMINAL_SESSION_STATE_CLOSED, "closed", ""
	}
	return TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED, "disconnected", ""
}

func terminalConnectExitState(clientClosed bool) (TerminalSessionState, string, string) {
	if clientClosed {
		return TerminalSessionState_TERMINAL_SESSION_STATE_CLOSED, "closed", ""
	}
	return TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED, "exited", ""
}

func (r *TerminalResource) currentState() *Terminal {
	var current *Terminal
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = r.state.CloneVT()
	})
	if current == nil {
		return &Terminal{}
	}
	return current
}

func (r *TerminalResource) updateState(ctx context.Context, state TerminalSessionState, status, errMessage string) error {
	current := r.currentState()
	updated := current.CloneVT()
	updated.State = state
	updated.Status = status
	updated.Error = errMessage
	updated.UpdatedAt = timestamppb.New(time.Now())
	if err := updated.Validate(); err != nil {
		return err
	}
	if err := r.persistState(ctx, updated); err != nil {
		return errors.Wrap(err, "persist terminal state")
	}
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.state = updated.CloneVT()
		broadcast()
	})
	return nil
}

func (r *TerminalResource) persistState(ctx context.Context, state *Terminal) error {
	if r.engine == nil {
		return errors.New("terminal resource requires a world engine for status updates")
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

func readTerminalObject(ctx context.Context, objState world.ObjectState) (*Terminal, error) {
	var state *Terminal
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = UnmarshalTerminal(ctx, bcs)
		return uerr
	})
	return state, err
}

// _ is a type assertion
var _ SRPCTerminalResourceServiceServer = (*TerminalResource)(nil)
