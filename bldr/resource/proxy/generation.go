package resource_proxy

import (
	"context"
	"io"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
)

var (
	// errReplayFailed is a call the proxy sent again that a new upstream refused.
	errReplayFailed = errors.New("the server refused a call this connection depends on")
	// errGenerationEnded ends a call whose generation ended first.
	errGenerationEnded = errors.New("resource client ended")
)

// generation is one ResourceClient stream of the client. It holds a root ID of
// the proxy's own, binds it and the children beneath it to resources on each
// upstream in turn, and carries the client's controls and the upstream's
// releases across the binding.
type generation struct {
	// p is the proxy serving the generation.
	p *Proxy
	// client is the client's ResourceClient stream.
	client resource.SRPCResourceService_ResourceClientStream
	// id is the client handle ID the proxy issued.
	id uint32
	// root is the root resource ID the proxy issued.
	root uint32
	// ctx ends with the generation.
	ctx context.Context
	// cancel ends the generation.
	cancel context.CancelFunc

	// bcast guards the fields below and every send to client, so control
	// acknowledgements reach the client in the order of its controls.
	bcast broadcast.Broadcast
	// bound is the binding calls use, or nil while the generation binds.
	bound *binding
	// controls holds the control IDs the upstream has not acknowledged,
	// oldest first: the client's, or 0 for one the proxy sent itself.
	controls []uint32
	// replays holds the last successful call of each Replay method.
	replays []replay
	// children holds the live children in the order they were opened, so
	// each parent opens again before its children.
	children []*child
}

// binding is a generation's root on one upstream.
type binding struct {
	// up is the upstream.
	up *upstream
	// stream is the generation's ResourceClient stream on up.
	stream resource.SRPCResourceService_ResourceClientClient
	// root is the root resource ID up issued.
	root uint32
	// next is the last control ID sent on stream.
	next uint32
	// children holds the generation's children on up by their upstream ID.
	// The generation's bcast guards it.
	children map[uint32]*child
}

// place moves ch to b as the upstream child up. The caller holds the
// generation's bcast.
func (b *binding) place(ch *child, up uint32) {
	ch.up, ch.on = up, b
	b.children[up] = ch
}

// replay is a call the proxy sends again on each new upstream.
type replay struct {
	// service and method name the call.
	service, method string
	// packets are the srpc packets the client sent.
	packets [][]byte
}

// newGeneration constructs a generation serving client under id and root.
func newGeneration(p *Proxy, client resource.SRPCResourceService_ResourceClientStream, id, root uint32) *generation {
	ctx, cancel := context.WithCancel(client.Context())
	return &generation{p: p, client: client, id: id, root: root, ctx: ctx, cancel: cancel}
}

// serve sends the client its root and carries the generation until the
// client ends it or the generation retires.
func (g *generation) serve() error {
	// Publish the proxy's identities for the generation.
	defer g.cancel()
	err := g.client.Send(&resource.ResourceClientResponse{Body: &resource.ResourceClientResponse_Init{
		Init: &resource.ResourceClientInit{ClientHandleId: g.id, RootResourceId: g.root},
	}})
	if err != nil {
		return err
	}

	// Carry the client's controls and the upstream binding side by side.
	errs := make(chan error, 2)
	go func() { errs <- g.serveControls() }()
	go func() { errs <- g.keepBound() }()
	return <-errs
}

// serveControls forwards the client's controls until its stream ends.
func (g *generation) serveControls() error {
	for {
		req, err := g.client.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := g.control(req); err != nil {
			return err
		}
	}
}

// control applies one Adopt or Release. On a binding it goes upstream with the
// upstream's IDs; without one there is no upstream resource to change, so the
// proxy acknowledges it at once and the next binding adopts what the client
// holds.
func (g *generation) control(req *resource.ResourceClientRequest) error {
	// Read the resource the control names.
	var id uint32
	var adopt bool
	switch body := req.GetBody().(type) {
	case *resource.ResourceClientRequest_Adopt:
		id, adopt = body.Adopt.GetResourceId(), true
	case *resource.ResourceClientRequest_Release:
		id = body.Release.GetResourceId()
	default:
		return errors.New("expected ResourceClient control")
	}

	// Hold the generation so the control keeps its place in the order.
	locked := g.bcast.Lock()
	defer locked.Unlock()

	// Map the resource to the binding's.
	b := g.bound
	var up uint32
	switch ch := g.childLocked(id); {
	case id == g.root:
		if b != nil {
			up = b.root
		}
	case ch != nil && adopt:
		ch.adopted = true
		up = ch.up
	case ch != nil:
		g.dropLocked(ch)
		up = ch.up
	default:
		// A resource the proxy does not hold is released already.
		b = nil
	}
	if b == nil {
		err := g.ackLocked(req.GetControlId())
		if err == nil && adopt && id != g.root && up == 0 {
			err = g.sendReleasedLocked(id)
		}
		return err
	}

	// Send it upstream. A failed send is the upstream's loss, which
	// acknowledges the control.
	g.sendLocked(b, req.GetControlId(), up, adopt)
	return nil
}

// sendLocked sends an Adopt or Release of up on b, acknowledging the client's
// control id when the upstream does, or nothing for id 0. The caller holds
// bcast, so controls reach the upstream in the order they are numbered.
func (g *generation) sendLocked(b *binding, id, up uint32, adopt bool) {
	// Number the control on b and queue the client's ID for its ack.
	b.next++
	g.controls = append(g.controls, id)

	// Send the control with the upstream's resource ID.
	out := &resource.ResourceClientRequest{ControlId: b.next}
	if adopt {
		out.Body = &resource.ResourceClientRequest_Adopt{Adopt: &resource.ResourceClientAdopt{ResourceId: up}}
	} else {
		out.Body = &resource.ResourceClientRequest_Release{Release: &resource.ResourceClientRelease{ResourceId: up}}
	}
	_ = b.stream.Send(out)
}

// keepBound binds the generation to each upstream in turn.
func (g *generation) keepBound() error {
	for {
		// Bind to the live upstream; a failed bind loses it.
		up, err := g.p.waitUpstream(g.ctx)
		if err != nil {
			return err
		}
		b, err := g.bind(up)
		if errors.Is(err, errReplayFailed) || g.ctx.Err() != nil {
			return err
		}
		if err != nil {
			g.p.lose(up)
			continue
		}
		g.p.bound(up)

		// Carry the binding until its upstream is lost. A generation the
		// client ended leaves the upstream to the others.
		_ = g.forward(b)
		g.unbind(b)
		if g.ctx.Err() != nil {
			return nil
		}
		g.p.lose(up)
	}
}

// bind opens the generation's root on up, opens each child again, and
// publishes the binding to calls.
func (g *generation) bind(up *upstream) (*binding, error) {
	// End the binding's context when up is lost.
	ctx, cancel := context.WithCancel(g.ctx)
	go func() {
		select {
		case <-up.lost:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Open a generation upstream and read its root.
	stream, err := up.service.ResourceClient(ctx)
	if err == nil {
		err = stream.Send(&resource.ResourceClientRequest{Body: &resource.ResourceClientRequest_Init{
			Init: &resource.ResourceClientInitRequest{},
		}})
	}
	var resp *resource.ResourceClientResponse
	if err == nil {
		resp, err = stream.Recv()
	}
	if err == nil && resp.GetInit() == nil {
		err = errors.New("expected ResourceClient init")
	}
	if err != nil {
		cancel()
		return nil, err
	}
	b := &binding{
		up:       up,
		stream:   stream,
		root:     resp.GetInit().GetRootResourceId(),
		children: make(map[uint32]*child),
	}

	// Send the Replay calls again before any other call reaches the root.
	var replays []replay
	g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		replays = slices.Clone(g.replays)
	})
	for _, r := range replays {
		if _, err := replayCall(ctx, up.service, b.root, r); err != nil {
			cancel()
			return nil, err
		}
	}

	// Open the children again and publish the binding.
	if err := g.rebind(ctx, b); err != nil {
		g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			g.controls = nil
		})
		cancel()
		return nil, err
	}
	return b, nil
}

// rebind opens each child again on b, parents first, and publishes b once
// every live child is on it. A child whose call the upstream refuses is
// released, and so are its children. It returns the upstream's transport
// failure; the children already moved open again on the next binding.
func (g *generation) rebind(ctx context.Context, b *binding) error {
	// Take the live children in the order they were opened. No child opens
	// while the generation is unbound, so the list is complete.
	var children []*child
	g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		children = slices.Clone(g.children)
	})

	// Open each child again beneath its parent, parents first.
	for _, ch := range children {
		if err := g.rebindChild(ctx, b, ch); err != nil {
			return err
		}
	}

	// Publish b to calls.
	g.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		g.bound = b
		broadcast()
	})
	return nil
}

// rebindChild opens ch again on b beneath its parent, which is already on b.
// A child whose parent is gone is released, and so is one the upstream
// refuses. It returns the upstream's transport failure.
func (g *generation) rebindChild(ctx context.Context, b *binding, ch *child) error {
	// Find the parent's ID on b, or release a child whose parent is gone.
	var parent uint32
	var err error
	g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		switch {
		case ch.released:
		case ch.parent == nil:
			parent = b.root
		case !ch.parent.released:
			parent = ch.parent.up
		default:
			err = g.releaseLocked(ch)
		}
	})
	if err != nil || parent == 0 {
		return err
	}

	// Send its Open call again beneath its parent. A refused child route
	// means the parent was released upstream.
	msg, err := replayCall(ctx, b.up.service, parent, ch.call)
	up, ok := readChildID(msg)
	var failure *resource.ResourceFailure
	refused := errors.Is(err, errReplayFailed) || (ch.parent != nil && errors.As(err, &failure)) || (err == nil && !ok)
	if err != nil && !refused {
		return err
	}

	// Move it to b, or release it.
	err = nil
	g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		switch {
		case ch.released:
			// The client released it meanwhile.
			if ok {
				g.sendLocked(b, 0, up, false)
			}
		case refused:
			err = g.releaseLocked(ch)
		default:
			b.place(ch, up)
			if ch.adopted {
				g.sendLocked(b, 0, up, true)
			}
		}
	})
	return err
}

// forward carries the binding's acknowledgements and releases to the client
// until the upstream stream ends.
func (g *generation) forward(b *binding) error {
	for {
		resp, err := b.stream.Recv()
		if err != nil {
			return err
		}
		switch body := resp.GetBody().(type) {
		case *resource.ResourceClientResponse_ControlAck:
			// The upstream acknowledges controls in order.
			var err error
			g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				if g.bound != b || len(g.controls) == 0 {
					return
				}
				id := g.controls[0]
				g.controls = g.controls[1:]
				if id != 0 {
					err = g.ackLocked(id)
				}
			})
			if err != nil {
				return err
			}
		case *resource.ResourceClientResponse_ResourceReleased:
			// A released root ends the binding; a released child is gone.
			id := body.ResourceReleased.GetResourceId()
			if id == b.root {
				return errors.New("upstream released the root")
			}
			var err error
			g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				if ch := b.children[id]; ch != nil {
					err = g.releaseLocked(ch)
				}
			})
			if err != nil {
				return err
			}
		}
	}
}

// unbind withdraws b from calls and acknowledges the client controls it left
// unanswered, since the upstream that would apply them is gone.
func (g *generation) unbind(b *binding) {
	g.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if g.bound == b {
			g.bound = nil
		}
		for _, id := range g.controls {
			if id != 0 {
				_ = g.ackLocked(id)
			}
		}
		g.controls = nil
		broadcast()
	})
}

// ackLocked acknowledges the client's control id. The caller holds bcast.
func (g *generation) ackLocked(id uint32) error {
	return g.client.Send(&resource.ResourceClientResponse{Body: &resource.ResourceClientResponse_ControlAck{
		ControlAck: &resource.ResourceClientControlAck{ControlId: id},
	}})
}

// open opens a route to the root, or to child ch, on the generation's binding,
// waiting while the generation binds. A refused root means the upstream ended
// the binding; a refused child is released, and its release follows.
func (g *generation) open(ctx context.Context, ch *child) (resource.SRPCResourceService_ResourceRpcClient, *binding, error) {
	var stale *binding
	for {
		// Wait for a binding other than the one that failed.
		var b *binding
		var target uint32
		var wait <-chan struct{}
		g.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if g.bound != stale {
				b = g.bound
			}
			switch {
			case b == nil:
			case ch == nil:
				target = b.root
			case !ch.released:
				target = ch.up
			}
			wait = getWaitCh()
		})
		if target == 0 && b != nil {
			return nil, nil, errReleased
		}
		if b == nil {
			select {
			case <-ctx.Done():
				return nil, nil, context.Cause(ctx)
			case <-g.ctx.Done():
				return nil, nil, errGenerationEnded
			case <-wait:
			}
			continue
		}

		// Open the route.
		route, failure, err := openRoute(ctx, b.up.service, target)
		if err == nil && failure == nil {
			return route, b, nil
		}
		if route != nil {
			_ = route.Close()
		}
		if ctx.Err() != nil {
			return nil, nil, context.Cause(ctx)
		}
		if g.ctx.Err() != nil {
			return nil, nil, errGenerationEnded
		}
		if failure != nil && ch != nil {
			return nil, nil, failure
		}
		g.p.lose(b.up)
		stale = b
	}
}

// record keeps a successful Replay call to send again on each new upstream,
// replacing the method's earlier call.
func (g *generation) record(r replay) {
	g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for i := range g.replays {
			if g.replays[i].service == r.service && g.replays[i].method == r.method {
				g.replays[i] = r
				return
			}
		}
		g.replays = append(g.replays, r)
	})
}

// replayCall sends r to target on service, waits for its result and returns
// its last message. A refusal returns errReplayFailed, and a refused route
// its ResourceFailure; a transport failure returns its own error, so the
// caller tries the next upstream.
func replayCall(ctx context.Context, service resource.SRPCResourceServiceClient, target uint32, r replay) ([]byte, error) {
	// Open the route and send the call as the client sent it.
	route, failure, err := openRoute(ctx, service, target)
	if err != nil {
		return nil, err
	}
	defer route.Close()
	if failure != nil {
		return nil, failure
	}
	for _, data := range r.packets {
		if err := route.Send(&resource.ResourceRpcPacket{Body: &resource.ResourceRpcPacket_Data{Data: data}}); err != nil {
			return nil, err
		}
	}

	// Read until the call settles.
	var msg []byte
	for {
		pkt, err := route.Recv()
		if err != nil {
			return nil, err
		}
		var p srpc.Packet
		if p.UnmarshalVT(pkt.GetData()) != nil {
			continue
		}
		data := p.GetCallData()
		if len(data.GetData()) != 0 {
			msg = data.GetData()
		}
		if !settles(data) {
			continue
		}
		if data.GetError() != "" || data.GetErrorCode() != srpc.ErrorCode_ERROR_CODE_UNKNOWN {
			return nil, errors.Wrapf(errReplayFailed, "%s.%s: %s", r.service, r.method, data.GetError())
		}
		_ = route.CloseSend()
		return msg, nil
	}
}
