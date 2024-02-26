package valuelist

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
)

// WatchDirectiveResponse is a response message type.
type WatchDirectiveResponse[T any] interface {
	// GetValueId returns the value ID.
	GetValueId() uint32
	// GetIdle gets the idle field.
	GetIdle() bool
	// GetRemoved gets the removed field.
	GetRemoved() bool
	// GetValue gets the value field.
	GetValue() T

	// SetValueId sets the value id field.
	SetValueId(id uint32)
	// SetIdle sets the idle field.
	SetIdle(idle bool)
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
	var mtx sync.Mutex
	var bcast broadcast.Broadcast
	var sendQueue []R
	waitCh := bcast.GetWaitCh()

	queueSend := func(msg R) {
		mtx.Lock()
		for i := 0; i < len(sendQueue); i++ {
			// remove any referring to the same value id
			smsg := sendQueue[i]
			if smsg.GetValueId() == msg.GetValueId() {
				sendQueue = append(sendQueue[:i], sendQueue[i+1:]...)
			}
		}
		sendQueue = append(sendQueue, msg)
		bcast.Broadcast()
		mtx.Unlock()
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

	defer di.AddIdleCallback(func(_ []error) {
		msg := ctor()
		msg.SetIdle(true)
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

		mtx.Lock()
		writeQueue := sendQueue
		sendQueue = nil
		waitCh = bcast.GetWaitCh()
		mtx.Unlock()
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
// returnOnIdle returns nil if the result is marked idle
func WatchDirectiveViaStream[T any, R WatchDirectiveResponse[T]](
	ctx context.Context,
	strm srpc.StreamRecv[R],
	hnd directive.ValueHandler,
	idle func(),
	returnOnIdle bool,
) error {
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
			if msg.GetRemoved() {
				_, _ = hnd.RemoveValue(valueID)
			} else {
				_, _ = hnd.AddValue(msg.GetValue())
			}
		}

		if msg.GetIdle() {
			if idle != nil {
				idle()
			}
			if returnOnIdle {
				return nil
			}
		}
	}
}
