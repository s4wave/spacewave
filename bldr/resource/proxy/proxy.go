package resource_proxy

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
)

// Dialer opens a client of the upstream resource service. The proxy calls it
// again each time the upstream it returned is lost.
type Dialer func(ctx context.Context) (resource.SRPCResourceServiceClient, error)

// rootIDBase and childIDBase mark the root and child IDs the proxy issues. An
// upstream server numbers its resources from 1, so the high bits keep a proxy
// ID from naming an upstream resource.
const (
	rootIDBase  = 1 << 31
	childIDBase = 1 << 30
)

// The proxy waits redialMin after the first upstream loss, doubling up to
// redialMax while each new upstream fails before a client binds to it.
const (
	redialMin = 100 * time.Millisecond
	redialMax = 5 * time.Second
)

// errAttach rejects ResourceAttach, which the proxy does not forward.
var errAttach = errors.New("resource proxy does not forward ResourceAttach")

// Proxy serves one client's Resource SDK in front of an upstream that can be
// replaced. Each ResourceClient generation the client opens gets a root of
// the proxy's own, bound in turn to a root on each upstream. Each child an
// Open call adds gets an ID of the proxy's own too, mapped in turn to the
// child that call adds again on each upstream.
type Proxy struct {
	// ctx bounds the proxy and its dials.
	ctx context.Context
	// rules decides what each call on a root does when the upstream changes.
	rules Rules
	// dial opens a new upstream.
	dial Dialer

	// bcast guards the fields below and wakes their waiters.
	bcast broadcast.Broadcast
	// up is the live upstream, or nil while the proxy dials one.
	up *upstream
	// dialing is set while a dial runs.
	dialing bool
	// losses counts upstreams lost since a generation last bound to one.
	losses int
	// generations holds the live generations by their root ID.
	generations map[uint32]*generation
	// nextGeneration is the last generation number issued.
	nextGeneration uint32
	// children holds the live children by their proxy ID.
	children map[uint32]*child
	// nextChild is the last child number issued.
	nextChild uint32
}

// upstream is one connection to the upstream resource service.
type upstream struct {
	// service opens the upstream's ResourceClient and ResourceRpc streams.
	service resource.SRPCResourceServiceClient
	// lost closes when the proxy gives up on this upstream.
	lost chan struct{}
}

// NewProxy constructs a proxy that serves its client from the upstreams dial
// opens, applying rules to the calls open when one is replaced. It dials
// when a generation first needs an upstream and stops when ctx ends.
func NewProxy(ctx context.Context, rules Rules, dial Dialer) *Proxy {
	return &Proxy{
		ctx:         ctx,
		rules:       rules,
		dial:        dial,
		generations: make(map[uint32]*generation),
		children:    make(map[uint32]*child),
	}
}

// ResourceClient serves one client generation from the proxy's own root.
func (p *Proxy) ResourceClient(strm resource.SRPCResourceService_ResourceClientStream) error {
	// Require Init before allocating the generation.
	first, err := strm.Recv()
	if err != nil {
		return err
	}
	if first.GetInit() == nil {
		return errors.New("expected ResourceClient init packet")
	}

	// Register the generation under a root ID of the proxy's own.
	var g *generation
	p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		p.nextGeneration++
		g = newGeneration(p, strm, p.nextGeneration, rootIDBase|p.nextGeneration)
		p.generations[g.root] = g
	})
	defer p.retire(g)
	return g.serve()
}

// ResourceRpc serves one call on a root or on a child beneath it. The route
// is accepted at once and held until its generation binds.
func (p *Proxy) ResourceRpc(strm resource.SRPCResourceService_ResourceRpcStream) error {
	// Read the route's resource.
	first, err := strm.Recv()
	if err != nil {
		return err
	}
	init := first.GetInit()
	if init == nil {
		return errors.New("expected ResourceRpc init")
	}

	// Find its generation; an ID the proxy did not issue names nothing.
	id := init.GetResourceId()
	var g *generation
	var ch *child
	p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		switch {
		case id&rootIDBase != 0:
			g = p.generations[id]
		case id&childIDBase != 0:
			if ch = p.children[id]; ch != nil {
				g = ch.g
			}
		}
	})
	if g == nil {
		return strm.Send(newAck(resource.ErrResourceOrClientReleased))
	}
	if err := strm.Send(newAck(nil)); err != nil {
		return err
	}
	return newCall(strm, g, ch).run()
}

// addChild issues ch its proxy ID and registers it for calls.
func (p *Proxy) addChild(ch *child) {
	p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		p.nextChild++
		ch.id = childIDBase | p.nextChild
		p.children[ch.id] = ch
	})
}

// dropChildren withdraws ids from calls.
func (p *Proxy) dropChildren(ids ...uint32) {
	p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, id := range ids {
			delete(p.children, id)
		}
	})
}

// retire withdraws an ended generation and its children from calls.
func (p *Proxy) retire(g *generation) {
	ids := g.childIDs()
	p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		delete(p.generations, g.root)
		for _, id := range ids {
			delete(p.children, id)
		}
	})
}

// ResourceAttach is not forwarded.
func (p *Proxy) ResourceAttach(resource.SRPCResourceService_ResourceAttachStream) error {
	return errAttach
}

// waitUpstream returns the live upstream, dialing one when there is none.
func (p *Proxy) waitUpstream(ctx context.Context) (*upstream, error) {
	for {
		// Take the live upstream, or start the one dial that replaces it.
		var up *upstream
		var wait <-chan struct{}
		p.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			up = p.up
			if up == nil && !p.dialing {
				p.dialing = true
				go p.redial(p.losses)
			}
			wait = getWaitCh()
		})
		if up != nil {
			return up, nil
		}

		// Wait for the dial to publish its upstream.
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-p.ctx.Done():
			return nil, context.Cause(p.ctx)
		case <-wait:
		}
	}
}

// redial dials until it publishes a new upstream or the proxy ends. It waits
// first when earlier upstreams were lost before a generation bound.
func (p *Proxy) redial(losses int) {
	delay := time.Duration(0)
	if losses > 0 {
		delay = min(redialMin<<min(losses-1, 6), redialMax)
	}
	for {
		// Wait out the backoff.
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-p.ctx.Done():
				timer.Stop()
				p.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
					p.dialing = false
					broadcast()
				})
				return
			case <-timer.C:
			}
		}

		// Publish the upstream once it dials.
		service, err := p.dial(p.ctx)
		if err == nil {
			p.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				p.up = &upstream{service: service, lost: make(chan struct{})}
				p.dialing = false
				broadcast()
			})
			return
		}
		delay = min(max(delay*2, redialMin), redialMax)
	}
}

// lose gives up on up, so the next waiter dials its replacement.
func (p *Proxy) lose(up *upstream) {
	p.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if p.up != up {
			return
		}
		close(up.lost)
		p.up = nil
		p.losses++
		broadcast()
	})
}

// bound records that a generation bound to up, which resets the backoff.
func (p *Proxy) bound(up *upstream) {
	p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if p.up == up {
			p.losses = 0
		}
	})
}

// openRoute opens a ResourceRpc route to id on service and returns the
// upstream's refusal, if any.
func openRoute(
	ctx context.Context,
	service resource.SRPCResourceServiceClient,
	id uint32,
) (resource.SRPCResourceService_ResourceRpcClient, *resource.ResourceFailure, error) {
	route, err := service.ResourceRpc(ctx)
	if err != nil {
		return nil, nil, err
	}
	err = route.Send(&resource.ResourceRpcPacket{Body: &resource.ResourceRpcPacket_Init{
		Init: &resource.ResourceRpcInit{ResourceId: id},
	}})
	var ack *resource.ResourceRpcPacket
	if err == nil {
		ack, err = route.Recv()
	}
	if err == nil && ack.GetAck() == nil {
		err = errors.New("expected ResourceRpc acknowledgement")
	}
	if err != nil {
		_ = route.Close()
		return nil, nil, err
	}
	return route, ack.GetAck().GetFailure(), nil
}

// newAck returns a route acknowledgement that refuses with failure, or
// accepts when failure is nil.
func newAck(failure *resource.ResourceFailure) *resource.ResourceRpcPacket {
	return &resource.ResourceRpcPacket{Body: &resource.ResourceRpcPacket_Ack{
		Ack: &resource.ResourceRpcAck{Failure: failure},
	}}
}

// _ is a type assertion.
var _ resource.SRPCResourceServiceServer = (*Proxy)(nil)
