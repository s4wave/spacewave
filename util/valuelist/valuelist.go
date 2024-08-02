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
	var bcast broadcast.Broadcast
	var sendQueue []R

	var waitCh <-chan struct{}
	bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

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

	var wasIdle atomic.Bool
	defer di.AddIdleCallback(func(isIdle bool, _ []error) {
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

var debugInst atomic.Int32

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

	instID := debugInst.Add(1)
	le.Infof("WatchDirectiveViaStream: starting %v", instID)
	defer le.Infof("WatchDirectiveViaStream: exiting %v", instID)

	for {
		if ctx.Err() != nil {
			return context.Canceled
		}

		msg, err := strm.Recv()
		if err != nil {
			return err
		}

		le.Infof("WatchDirectiveViaStream: got msg %v: valueID(%v) removed(%v) idle(%v)", instID, msg.GetValueId(), msg.GetRemoved(), msg.GetIdle())

		valueID := msg.GetValueId()
		if valueID != 0 {
			if msg.GetRemoved() {
				_, _ = hnd.RemoveValue(valueID)
			} else {
				_, _ = hnd.AddValue(msg.GetValue())
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
