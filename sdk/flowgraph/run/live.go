package flowgraph_run

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	"github.com/sirupsen/logrus"
)

// ErrActivationEnded is returned when a run no longer holds a running
// activation under the given Task key: the activation ended and the run
// returned its unacknowledged live inputs to the arrivals, or it never
// existed. A Step that reads this has no inbox left to read or acknowledge.
var ErrActivationEnded = errors.New("activation is not running")

// runningActivation returns the running activation of a Step for a key, or
// nil when the Step has none.
func (r *runner) runningActivation(nodeID, key string) *s4wave_flowgraph.FlowgraphActivation {
	for _, activation := range r.run.GetActivations() {
		if !activation.GetDone() && activation.GetNodeId() == nodeID && activation.GetKey() == key {
			return activation
		}
	}
	return nil
}

// deliverLive moves each arrival of a keyed Step whose key has a running
// activation from the run's arrivals to that activation's inbox. The arrival
// gets the activation's next sequence number, in arrival order. It runs in the
// transaction that consumes the arrival, so an arrival is in exactly one of
// the arrivals and an inbox.
func (r *runner) deliverLive(nodeID string) {
	r.run.Arrivals = slices.DeleteFunc(r.run.Arrivals, func(arrival *s4wave_flowgraph.FlowgraphArrival) bool {
		// Keep arrivals at other nodes and keys with no running activation.
		if arrival.GetNodeId() != nodeID {
			return false
		}
		activation := r.runningActivation(nodeID, arrival.GetKey())
		if activation == nil {
			return false
		}

		// Move the arrival into the activation's inbox at its next sequence.
		activation.LastInputSeq++
		activation.Inbox = append(activation.Inbox, &s4wave_flowgraph.FlowgraphLiveInput{
			Seq:     activation.GetLastInputSeq(),
			Arrival: arrival,
		})
		return true
	})
}

// returnInbox returns the live inputs an ended activation never acknowledged
// to the run's arrivals, in sequence order and with their keys, and empties the
// inbox. The next readiness pass starts the successor that receives them.
func (r *runner) returnInbox(activation *s4wave_flowgraph.FlowgraphActivation) {
	for _, input := range activation.GetInbox() {
		r.run.Arrivals = append(r.run.Arrivals, input.GetArrival())
	}
	activation.Inbox = nil
}

// WatchLiveInputs watches the inbox of a running activation and calls cb with
// each live input it has not reported yet, in sequence order. The first call
// reports the inputs already waiting, so a Step restarted before it
// acknowledged an input reads that input again. A Step's exec passes the key of
// its run and the Task key of the activation it runs for, and acknowledges
// inputs it has handled with AckLiveInputs.
//
// cb runs on the watch loop, so a slow cb delays later calls but never loses
// an input. It may call AckLiveInputs. WatchLiveInputs returns the error cb
// returns, the context's error, world.ErrObjectNotFound when the run does not
// exist, or ErrActivationEnded once the activation has ended.
func WatchLiveInputs(
	ctx context.Context,
	ws world.WorldState,
	runKey, activationID string,
	cb func(inputs []*s4wave_flowgraph.FlowgraphLiveInput) error,
) error {
	var reported uint64
	loop := world_control.NewWatchLoop(nil, runKey, func(
		ctx context.Context,
		_ *logrus.Entry,
		_ world.WorldState,
		obj world.ObjectState,
		_ *bucket.ObjectRef,
		_ uint64,
	) (bool, error) {
		// Read the activation's inbox from the run object's revision.
		if obj == nil {
			return false, world.ErrObjectNotFound
		}
		inbox, err := readInbox(ctx, obj, activationID)
		if err != nil {
			return false, err
		}

		// Report the inputs past the last one reported.
		inbox = slices.DeleteFunc(inbox, func(input *s4wave_flowgraph.FlowgraphLiveInput) bool {
			return input.GetSeq() <= reported
		})
		if len(inbox) != 0 {
			reported = inbox[len(inbox)-1].GetSeq()
			if err := cb(inbox); err != nil {
				return false, err
			}
		}
		return true, nil
	})
	return loop.Execute(ctx, ws)
}

// readInbox reads the inbox of the running activation with the given Task key
// from a run object, or ErrActivationEnded when the run holds none.
func readInbox(ctx context.Context, obj world.ObjectState, activationID string) ([]*s4wave_flowgraph.FlowgraphLiveInput, error) {
	// Find the running activation and copy its inbox.
	var inbox []*s4wave_flowgraph.FlowgraphLiveInput
	var found bool
	_, _, err := world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
		run, err := s4wave_flowgraph.UnmarshalFlowgraphRun(ctx, cursor)
		if err != nil {
			return err
		}
		for _, activation := range run.GetActivations() {
			if activation.GetTaskKey() == activationID && !activation.GetDone() {
				inbox, found = activation.CloneVT().GetInbox(), true
			}
		}
		return nil
	})
	if err == nil && !found {
		err = ErrActivationEnded
	}
	return inbox, err
}

// AckLiveInputs acknowledges the live inputs of a running activation up to and
// including the sequence number: it removes them from the inbox, so the run
// does not return them when the activation ends. An input the Step took but
// did not acknowledge before its activation ended starts a successor, so a
// Step acknowledges an input only once it has handled it. It changes nothing
// when no input is that old. It returns ErrActivationEnded when the activation
// is no longer running; the run has then returned the inputs to the arrivals.
// Call it in a write transaction.
func AckLiveInputs(
	ctx context.Context,
	ws world.WorldState,
	runKey, activationID string,
	upToSeq uint64,
) error {
	// Read the run and find the running activation.
	obj, run, _, err := readRun(ctx, ws, runKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	next := run.CloneVT()
	var activation *s4wave_flowgraph.FlowgraphActivation
	for _, candidate := range next.GetActivations() {
		if candidate.GetTaskKey() == activationID && !candidate.GetDone() {
			activation = candidate
		}
	}
	if activation == nil {
		return ErrActivationEnded
	}

	// Remove the acknowledged inputs and write the run when any went.
	inbox := slices.DeleteFunc(activation.Inbox, func(input *s4wave_flowgraph.FlowgraphLiveInput) bool {
		return input.GetSeq() <= upToSeq
	})
	if len(inbox) == len(activation.GetInbox()) {
		return nil
	}
	activation.Inbox = inbox
	return writeRun(ctx, ws, runKey, obj, func(cursor *block.Cursor) {
		cursor.SetBlock(next, true)
	})
}
