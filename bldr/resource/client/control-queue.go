package resource_client

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/bldr/resource"
)

// resourceControlQueue serializes writes for one ResourceClient generation.
type resourceControlQueue struct {
	// stream accepts every control for this generation
	stream resource.SRPCResourceService_ResourceClientClient

	onFailure func(error)
	onDone    func()
	firstSent chan error

	// bcast guards the queue state below
	bcast   broadcast.Broadcast
	items   []*resource.ResourceClientRequest
	closing bool
	retired bool
}

func newResourceControlQueue(
	stream resource.SRPCResourceService_ResourceClientClient,
	onFailure func(error),
	onDone func(),
) *resourceControlQueue {
	q := &resourceControlQueue{
		stream:    stream,
		onFailure: onFailure,
		onDone:    onDone,
		firstSent: make(chan error, 1),
	}
	go q.run()
	return q
}

func (q *resourceControlQueue) enqueue(req *resource.ResourceClientRequest) bool {
	if req == nil {
		return false
	}

	var accepted bool
	q.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if q.closing || q.retired {
			return
		}
		q.items = append(q.items, req)
		accepted = true
		broadcast()
	})
	return accepted
}

func (q *resourceControlQueue) finish() {
	q.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if q.retired || q.closing {
			return
		}
		q.closing = true
		broadcast()
	})
}

func (q *resourceControlQueue) retire(err error) {
	// Retire the queue and discard controls that were never transmitted.
	var retired bool
	q.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if q.retired {
			return
		}
		q.retired = true
		q.closing = true
		q.items = nil
		retired = true
		broadcast()
	})

	// Notify the generation after the queue state is final.
	if retired && q.onFailure != nil {
		q.onFailure(err)
	}
}

func (q *resourceControlQueue) run() {
	defer func() {
		_ = q.stream.CloseSend()
		if q.onDone != nil {
			q.onDone()
		}
	}()

	first := true
	for {
		// Snapshot one control or the matching wait channel.
		var req *resource.ResourceClientRequest
		var waitCh <-chan struct{}
		var done bool
		q.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if len(q.items) != 0 {
				req = q.items[0]
				q.items[0] = nil
				q.items = q.items[1:]
				if len(q.items) == 0 {
					q.items = nil
				}
				return
			}
			if q.closing || q.retired {
				done = true
				return
			}
			waitCh = getWaitCh()
		})
		if done {
			return
		}
		if req == nil {
			select {
			case <-waitCh:
				continue
			case <-q.stream.Context().Done():
				q.retire(context.Cause(q.stream.Context()))
				return
			}
		}

		// Send each control before dequeuing the next one.
		err := q.stream.Send(req)
		if first {
			first = false
			q.firstSent <- err
		}
		if err != nil {
			q.retire(err)
			return
		}
	}
}
