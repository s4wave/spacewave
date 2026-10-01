package valuelist

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/sirupsen/logrus"
)

// WatchDirectiveResponse is a response message type.
type WatchDirectiveResponse[T any] interface {
	// GetValueId returns the value ID.
	GetValueId() uint32
	// GetIdle gets the idle field.
	//
	// 0 = no change
	// 1 = not idle
	// 2 = idle
	GetIdle() uint32
	// GetRemoved gets the removed field.
	GetRemoved() bool
	// GetValue gets the value field.
	GetValue() T

	// SetValueId sets the value id field.
	SetValueId(id uint32)
	// SetIdle sets the idle field.
	//
	// 0 = no change
	// 1 = not idle
	// 2 = idle
	SetIdle(idle uint32)
	// SetRemoved sets the removed field.
	SetRemoved(removed bool)
	// SetValue sets the value field.
	SetValue(val T)
}

// WatchDirective adds a directive and watches the list of values, sending
// update messages over the stream.
//
// T is the type of the value and R is the type of the response message.
// errCh is an optional error channel to interrupt the operation.
func WatchDirective[T any, R WatchDirectiveResponse[T]](
	ctx context.Context,
	b bus.Bus,
	dir directive.Directive,
	ctor func() R,
	send func(msg R) error,
	errCh <-chan error,
) error {
	// Create the broadcast and an initial send queue.
	var bcast broadcast.Broadcast
	var sendQueue []R

	// Snapshot the initial wait channel under the broadcast lock.
	var waitCh <-chan struct{}
	bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

	// Queue send messages, replacing any queued message with the same value ID.
	queueSend := func(msg R) {
		bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			for i := 0; i < len(sendQueue); i++ {
				// remove any referring to the same value id
				smsg := sendQueue[i]
				if smsg.GetValueId() == msg.GetValueId() {
					sendQueue = append(sendQueue[:i], sendQueue[i+1:]...)
				}
			}
			sendQueue = append(sendQueue, msg)
			broadcast()
		})
	}

	// Add the directive with callbacks that queue added and removed values.
	di, dirRef, err := b.AddDirective(
		dir,
		bus.NewCallbackHandler(
			func(av directive.AttachedValue) {
				v, ok := av.GetValue().(T)
				if ok {
					msg := ctor()
					msg.SetValueId(av.GetValueID())
					msg.SetValue(v)
					msg.SetRemoved(false)
					queueSend(msg)
				}
			}, func(av directive.AttachedValue) {
				_, ok := av.GetValue().(T)
				if ok {
					msg := ctor()
					msg.SetValueId(av.GetValueID())
					msg.SetRemoved(true)
					queueSend(msg)
				}
			},
			nil,
		),
	)
	if err != nil {
		return err
	}
	defer dirRef.Release()

	// Queue an idle-state message whenever the directive idle state changes.
	var wasIdle atomic.Bool
	defer di.AddIdleCallback(func(isIdle bool, _ []error) {
		// Skip repeated idle callbacks and encode the new idle state.
		if wasIdle.Swap(isIdle) == isIdle {
			return
		}
		idleVal := uint32(1)
		if isIdle {
			idleVal = 2
		}
		msg := ctor()
		msg.SetIdle(idleVal)
		queueSend(msg)
	})()

	// Wait for queue updates and forward each queued message to the stream.
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			return err
		case <-waitCh:
		}

		var writeQueue []R
		bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			writeQueue = sendQueue
			sendQueue = nil
			waitCh = getWaitCh()
		})
		for _, msg := range writeQueue {
			if err := send(msg); err != nil {
				return err
			}
		}
	}
}

// WatchDirectiveViaStream resolves a directive by watching a value stream.
//
// T is the type of the value and R is the type of the response message.
// idle is an optional callback when the result is marked idle
// the stream will be closed on return
// returnOnIdle returns nil if the result is marked idle
func WatchDirectiveViaStream[T any, R WatchDirectiveResponse[T]](
	ctx context.Context,
	strm srpc.StreamRecv[R],
	hnd directive.ValueHandler,
	idle func(isIdle bool),
	returnOnIdle bool,
	le *logrus.Entry,
) (rerr error) {
	defer func() {
		_ = strm.CloseSend()
		if err := strm.Close(); rerr == nil && err != nil {
			rerr = err
		}
	}()

	// localIDs maps remote value IDs to the IDs assigned by hnd.
	localIDs := make(map[uint32]uint32)
	for {
		if ctx.Err() != nil {
			return context.Canceled
		}

		msg, err := strm.Recv()
		if err != nil {
			return err
		}

		valueID := msg.GetValueId()
		if valueID != 0 {
			if localID, ok := localIDs[valueID]; ok {
				delete(localIDs, valueID)
				_, _ = hnd.RemoveValue(localID)
			}
			if !msg.GetRemoved() {
				if localID, accepted := hnd.AddValue(msg.GetValue()); accepted {
					localIDs[valueID] = localID
				}
			}
		}

		idleVal := msg.GetIdle()
		if idleVal != 0 {
			isIdle := idleVal != 1
			if idle != nil {
				idle(isIdle)
			}
			if returnOnIdle && isIdle {
				return nil
			}
		}
	}
}
