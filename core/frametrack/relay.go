package frametrack

import (
	"context"
	"sync"
)

// Relay forwards the newest group's decodable prefix to one ordered consumer.
type Relay struct {
	// packets carries each accepted picture to the waiting consumer.
	packets chan *FramePacket
	// sender cuts a group whose consumer has fallen behind.
	sender *Sender

	// mtx serializes group selection and admission to the sender.
	mtx sync.Mutex
	// newest is the latest source group admitted to the relay.
	newest uint64

	// last is the last group delivered to the one consumer.
	last uint64
}

// NewRelay relays groups while ctx is live. window counts pending pictures and
// the active handoff to the consumer.
func NewRelay(ctx context.Context, window int) (*Relay, error) {
	// Hand each picture to the consumer without an additional queue or encode.
	r := &Relay{packets: make(chan *FramePacket)}
	sender, err := NewSender(ctx, "", window, r.writeGroup, nil)
	if err != nil {
		return nil, err
	}
	r.sender = sender
	return r, nil
}

// ReadGroup reads the newest source group until it ends or the relay cuts it.
// It can run concurrently for different groups on the same subscription.
func (r *Relay) ReadGroup(_ context.Context, group *GroupReader) {
	header := group.Header()
	for {
		// Admit only complete pictures of the current or a newer group.
		frame, err := group.ReadFrame()
		if err != nil {
			return
		}

		// Hold this ordering decision through the nonblocking queue append.
		r.mtx.Lock()
		queued := false
		sequence := header.GetGroupSequence()
		if sequence > r.newest || (sequence == r.newest && !frame.GetKeyframe()) {
			r.newest = sequence
			queued = r.sender.Send(header, frame)
		}
		r.mtx.Unlock()
		if !queued {
			return
		}
	}
}

// ReadFrame takes the next picture or reports why ctx ended.
// Only keyframes carry the group's decoding parameters.
// One consumer calls ReadFrame in delivery order.
func (r *Relay) ReadFrame(ctx context.Context) (*FramePacket, error) {
	for {
		// Retiring a writer can race with its final handoff. Only the newest
		// delivered group may contribute pictures from that point on.
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case packet := <-r.packets:
			sequence := packet.GetHeader().GetGroupSequence()
			if sequence >= r.last {
				r.last = sequence
				if !packet.GetFrame().GetKeyframe() {
					packet.Header = nil
				}
				return packet, nil
			}
		}
	}
}

// writeGroup retires a blocked handoff as soon as a newer group opens.
func (r *Relay) writeGroup(ctx context.Context, header *GroupHeader, drain func(func(*Frame) error) error) error {
	return drain(func(frame *Frame) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r.packets <- &FramePacket{Header: header, Frame: frame}:
			return nil
		}
	})
}
