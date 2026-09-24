package sobject_sync

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// leanSyncAuthorityRead supplies a primitive state observation or wait failure.
type leanSyncAuthorityRead struct {
	// state is a fresh pointer delivered through the real state container.
	state *sobject.SOState
	// err ends the wait without an authority decision.
	err error
}

// leanSyncAuthorityWatch delivers a finite primitive trace through the real container API.
type leanSyncAuthorityWatch struct {
	// CContainer supplies all ordinary watch operations.
	*ccontainer.CContainer[*sobject.SOState]
	// reads includes trailing observations that a returned watcher must never consume.
	reads []leanSyncAuthorityRead
	// count records actual calls to WaitValueChange.
	count int
}

// WaitValueChange publishes the next distinct value or injects its primitive wait failure.
func (w *leanSyncAuthorityWatch) WaitValueChange(ctx context.Context, old *sobject.SOState, errCh <-chan error) (*sobject.SOState, error) {
	if w.count == len(w.reads) {
		return nil, errors.New("watch consumed beyond its finite trace")
	}
	read := w.reads[w.count]
	w.count++
	if read.err != nil {
		return nil, read.err
	}
	w.SetValue(read.state)
	return w.CContainer.WaitValueChange(ctx, old, errCh)
}

// leanSyncAuthorityStream observes deadline and close calls on a real pipe.
type leanSyncAuthorityStream struct {
	// authenticationStream records complete frames before their real transport write.
	*authenticationStream
	// failDeadline injects a transport deadline failure.
	failDeadline bool
	// deadline records the actual write deadline installed by the watcher.
	deadline time.Time
	// deadlineOK is the actual transport result, including an already closed pipe.
	deadlineOK bool
	// closed records transport closure.
	closed atomic.Bool
}

// SetWriteDeadline retains the requested bound even when the primitive fails.
func (s *leanSyncAuthorityStream) SetWriteDeadline(deadline time.Time) error {
	s.deadline = deadline
	if s.failDeadline {
		return errors.New("injected write deadline failure")
	}
	err := s.Conn.SetWriteDeadline(deadline)
	s.deadlineOK = err == nil
	return err
}

// Close records the actual closure before forwarding it to the pipe.
func (s *leanSyncAuthorityStream) Close() error {
	s.closed.Store(true)
	return s.Conn.Close()
}

// leanSyncFailure projects observed errors without supplying any authority decision.
func leanSyncFailure(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, context.Canceled):
		return 1
	case errors.Is(err, ErrAccessDenied):
		return 2
	case errors.Is(err, sobject.ErrConfigHistoryUnavailable):
		return 3
	default:
		return 4
	}
}

// TestLeanSyncAuthorityWatchConformance compares the actual watcher and its deferred cleanup.
func TestLeanSyncAuthorityWatchConformance(t *testing.T) {
	oracle := leanSyncOracle(t)
	var cases []leanSyncCase
	for seed := range uint64(4) {
		cases = append(cases, leanSyncAuthorityWatchCases(t, seed)...)
	}
	checkLeanSync(t, oracle, cases)
}

// FuzzLeanSyncAuthorityWatch varies real identities, roles and terminal primitive observations.
func FuzzLeanSyncAuthorityWatch(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(23))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanSync(t, leanSyncOracle(t), leanSyncAuthorityWatchCases(t, seed))
	})
}

// leanSyncAuthorityWatchCases includes blocked denial writes and unread regrants after termination.
func leanSyncAuthorityWatchCases(t *testing.T, seed uint64) []leanSyncCase {
	t.Helper()
	const objectID = "lean-sync-authority-watch"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, objectID, owner, reader)
	localID, err := peer.IDFromPrivateKey(owner)
	if err != nil {
		t.Fatal(err)
	}
	remoteID, err := peer.IDFromPrivateKey(reader)
	if err != nil {
		t.Fatal(err)
	}
	var cases []leanSyncCase
	for variant := range 13 {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		t.Cleanup(cancel)
		ownerCtx, ownerCancel := context.WithCancelCause(ctx)
		t.Cleanup(func() { ownerCancel(nil) })
		state := initial.CloneVT()
		var retainErr error
		var waitErr error
		switch variant {
		case 0:
			waitErr = context.Canceled
		case 1, 7, 8, 9:
			state.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
		case 2:
			state.Config.Participants[0].Role = sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
		case 3:
			state.Config.ConfigChainHash = nil
		case 4:
			state.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
			state.Config.ConfigChainHash = nil
		case 5:
			waitErr = errors.New("injected watch failure")
		case 6:
			retainErr = errors.New("injected retention failure")
		case 10:
			state.Config.Participants[1].Role = sobject.SOParticipantRole(-1 - int32(seed%32))
		case 11:
			state.Config.Participants = append(state.Config.Participants,
				participantCfg(remoteID.String(), sobject.SOParticipantRole_SOParticipantRole_UNKNOWN))
		case 12:
			state.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_WRITER
		}
		reads := []leanSyncAuthorityRead{{state: initial.CloneVT()}, {state: state, err: waitErr},
			{state: initial.CloneVT()}, {err: context.Canceled}}
		watch := &leanSyncAuthorityWatch{CContainer: ccontainer.NewCContainerVT[*sobject.SOState](nil), reads: reads}
		var retained, released atomic.Bool
		host := sobject.NewSOHost(ctx, func(context.Context, string, func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
			if retainErr != nil {
				return nil, nil, retainErr
			}
			retained.Store(true)
			return watch, func() { released.Store(true) }, nil
		}, nil, objectID)
		t.Cleanup(host.ClearContext)
		local := &SOSync{localObjectPeerID: localID, soHost: host}
		left, right := net.Pipe()
		observed := &leanSyncAuthorityStream{authenticationStream: &authenticationStream{Conn: left,
			messages: make(chan *SOSyncMessage, 2)}, failDeadline: variant == 7}
		stop := context.AfterFunc(ctx, func() { left.Close(); right.Close() })
		readDone := make(chan error, 1)
		if variant != 8 {
			go func() {
				message := &SOSyncMessage{}
				readDone <- stream_packet.NewSession(right, maxMessageSize).RecvMsg(message)
			}()
		}
		if variant == 9 {
			right.Close()
		}
		releasedBeforeCancel := false
		start := time.Now()
		err := local.watchAuthority(ownerCtx, observed, stream_packet.NewSession(observed, maxMessageSize), remoteID, func(err error) {
			releasedBeforeCancel = released.Load()
			ownerCancel(err)
		})
		finish := time.Now()
		if variant != 8 {
			select {
			case <-readDone:
			case <-ctx.Done():
				t.Fatal("authority watcher did not release the pipe reader")
			}
		}
		if !observed.deadline.IsZero() {
			clock := observed.deadline.Add(-250 * time.Millisecond)
			if clock.Before(start) || clock.After(finish) {
				t.Fatal("authority watcher write deadline has the wrong bound")
			}
		}
		if retained.Load() && !releasedBeforeCancel {
			t.Fatal("retained state was not released before owner cancellation")
		}
		if leanSyncFailure(err) != leanSyncFailure(context.Cause(ownerCtx)) {
			t.Fatal("watcher cancellation lost its returned cause")
		}

		var a fastjson.Arena
		request, expected, result, projected := a.NewObject(), a.NewObject(), a.NewObject(), a.NewArray()
		for i, read := range reads {
			value := a.NewObject()
			value.Set("participants", leanSyncParticipants(&a, read.state.GetConfig().GetParticipants()))
			value.Set("hash", a.NewString(hex.EncodeToString(read.state.GetConfig().GetConfigChainHash())))
			value.Set("error", a.NewNumberInt(leanSyncFailure(read.err)))
			projected.SetArrayItem(i, value)
		}
		request.Set("op", a.NewString("watchSyncAuthority"))
		request.Set("local", a.NewString(localID.String()))
		request.Set("remote", a.NewString(remoteID.String()))
		request.Set("retainError", a.NewNumberInt(leanSyncFailure(retainErr)))
		request.Set("deadlineOK", leanSyncBool(&a, observed.deadlineOK))
		request.Set("reads", projected)
		result.Set("reads", a.NewNumberInt(watch.count))
		result.Set("stopped", a.NewTrue())
		result.Set("cause", a.NewNumberInt(leanSyncFailure(err)))
		result.Set("deadline", leanSyncBool(&a, !observed.deadline.IsZero()))
		denial := len(observed.messages) != 0
		if denial {
			message := <-observed.messages
			if message.GetAuthorization() == nil || message.GetAuthorization().GetAccepted() || len(observed.messages) != 0 {
				t.Fatal("authority watcher sent data or more than one denial")
			}
		}
		result.Set("denial", leanSyncBool(&a, denial))
		result.Set("retained", leanSyncBool(&a, retained.Load()))
		result.Set("released", leanSyncBool(&a, released.Load()))
		result.Set("canceled", leanSyncBool(&a, ownerCtx.Err() != nil))
		result.Set("closed", leanSyncBool(&a, observed.closed.Load()))
		expected.Set("watcher", result)
		cases = append(cases, leanSyncCase{name: "authority watch seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
			request: request.MarshalTo(nil), expected: expected.MarshalTo(nil)})
		stop()
		cancel()
		left.Close()
		right.Close()
		host.ClearContext()
	}
	return cases
}
