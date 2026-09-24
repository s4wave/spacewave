package sobject_sync

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// leanSyncLoopStates applies a scheduled state update at the second actual authority read.
type leanSyncLoopStates struct {
	// CContainer supplies the actual state reads and change publication.
	*ccontainer.CContainer[*sobject.SOState]
	// next is the held value when an incoming frame is selected.
	next *sobject.SOState
	// reads counts actual GetValue calls.
	reads int
}

// GetValue publishes the scheduled update before the incoming authority check.
func (s *leanSyncLoopStates) GetValue() *sobject.SOState {
	s.reads++
	if s.reads == 2 {
		s.SetValue(s.next)
	}
	return s.CContainer.GetValue()
}

// leanSyncLoopRun selects one real production channel event and records its effects.
type leanSyncLoopRun struct {
	// current and next are distinct primitive reads around the selected event.
	current, next *sobject.SOState
	// message is the selected incoming frame.
	message *SOSyncMessage
	// event uses the oracle's cancel/expiry/change/handoff/write/read tags.
	event int
	// eventOK selects success or failure for writer and reader results.
	eventOK bool
	// drain optionally supplies explicit authorization after writer failure.
	drain *bool
	// denial records a real local denial packet attempt.
	denial bool
	// handed is the frame actually sent through the writer channel.
	handed *SOSyncMessage
}

// run drives the real select with only the requested channel made ready.
func (r *leanSyncLoopRun) run(t *testing.T, x *syncExchange, le *logrus.Entry) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	stop := context.AfterFunc(ctx, func() { left.Close(); right.Close() })
	defer stop()
	observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 2)}
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		remote := stream_packet.NewSession(right, maxMessageSize)
		for {
			if err := remote.RecvMsg(&SOSyncMessage{}); err != nil {
				return
			}
		}
	}()
	defer func() { left.Close(); right.Close(); <-readerDone }()

	states := &leanSyncLoopStates{CContainer: ccontainer.NewCContainerVT[*sobject.SOState](r.current), next: r.next}
	incoming := make(chan syncIncoming, 2)
	outbound := make(chan *SOSyncMessage)
	sent := make(chan error, 1)
	changed := make(chan struct{}, 1)
	var driverDone chan struct{}
	defer func() {
		cancel()
		if driverDone != nil {
			<-driverDone
		}
	}()
	switch r.event {
	case 0:
		cancel()
	case 1:
		// The fixture's expired pinned deadline is the only ready event.
	case 2:
		changed <- struct{}{}
	case 3:
		outbound = make(chan *SOSyncMessage, 1)
	case 4:
		sent = make(chan error)
		driverDone = make(chan struct{})
		go func() {
			defer close(driverDone)
			var err error
			if !r.eventOK {
				err = errors.New("injected writer failure")
			}
			select {
			case sent <- err:
			case <-ctx.Done():
				return
			}
			if err != nil {
				drained := syncIncoming{err: errors.New("injected drain completion")}
				if r.drain != nil {
					drained = syncIncoming{message: &SOSyncMessage{Body: &SOSyncMessage_Authorization{
						Authorization: &SOSyncAuthorization{Accepted: *r.drain}}}}
				}
				select {
				case incoming <- drained:
				case <-ctx.Done():
				}
			}
		}()
	case 5:
		var err error
		if !r.eventOK {
			err = errors.New("injected reader failure")
		}
		incoming <- syncIncoming{message: r.message, err: err}
	}
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	err := x.advance(ctx, le, stream_packet.NewSession(observed, maxMessageSize), states, incoming, outbound, sent, changed, timer)
	if errors.Is(err, context.DeadlineExceeded) && r.event != 1 {
		t.Fatal("loop did not complete its selected event")
	}
	select {
	case r.handed = <-outbound:
	default:
	}
	if len(observed.messages) != 0 {
		message := <-observed.messages
		if message.GetAuthorization() == nil || message.GetAuthorization().GetAccepted() || len(observed.messages) != 0 {
			t.Fatal("loop authority rejection sent data or more than one denial")
		}
		r.denial = true
	}
	return err
}

// TestLeanSyncLoopConformance compares every selected-event branch with the production owner.
func TestLeanSyncLoopConformance(t *testing.T) {
	oracle := leanSyncOracle(t)
	var cases []leanSyncCase
	for seed := range uint64(4) {
		cases = append(cases, leanSyncExchangeCases(t, seed, true)...)
	}
	checkLeanSync(t, oracle, cases)
}

// FuzzLeanSyncLoop varies signed checkpoints and scheduled loop/authority observations.
func FuzzLeanSyncLoop(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(29))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanSync(t, leanSyncOracle(t), leanSyncExchangeCases(t, seed, true))
	})
}
