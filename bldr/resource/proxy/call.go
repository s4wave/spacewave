package resource_proxy

import (
	"bytes"
	"context"
	"io"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
)

// errStale ends an attempt whose answer came from a binding the generation
// left, so the call opens again on the next.
var errStale = errors.New("answered by a lost upstream")

// inbound is one packet read from a route, or the error that ended it.
type inbound struct {
	pkt *resource.ResourceRpcPacket
	err error
}

// call carries one client route to an upstream route, opening another
// upstream route when its upstream is lost and the call's Rule allows.
type call struct {
	// client is the client's route.
	client resource.SRPCResourceService_ResourceRpcStream
	// g is the generation the call runs in.
	g *generation
	// on is the child the call runs on, or nil for the root.
	on *child

	// start is the call's CallStart, once the client sent it.
	start *srpc.CallStart
	// rule is the Rule of start's method.
	rule Rule
	// sent holds the srpc packets the client sent, for a rule that sends
	// them again.
	sent [][]byte
	// clientDone is set once the client closed its side of the route.
	clientDone bool
	// forwarded is set once an upstream packet reached the client.
	forwarded bool
	// last is the last message forwarded to the client.
	last []byte
}

// newCall constructs a call on the client route to g's root, or to its child
// on when on is set.
func newCall(client resource.SRPCResourceService_ResourceRpcStream, g *generation, on *child) *call {
	return &call{client: client, g: g, on: on}
}

// run carries the call until it settles or the client leaves.
func (c *call) run() error {
	// Read the client's packets for the life of the call.
	ctx := c.client.Context()
	in := make(chan inbound)
	go pump(ctx, c.client, in)

	for again := false; ; again = true {
		// Open an upstream route, holding the call while there is none.
		up, b, err := c.g.open(ctx, c.on)
		if err != nil {
			if c.start == nil || ctx.Err() != nil {
				return err
			}
			return c.reset()
		}

		// Carry the call; resume it on a new route when its rule allows.
		settled, err := c.attempt(ctx, in, up, b, again)
		_ = up.Close()
		if settled {
			return err
		}
		if !c.resumes() {
			return c.reset()
		}
	}
}

// attempt carries the call over one upstream route on b. It reports whether
// the call settled; it returns unsettled when b's upstream was lost.
func (c *call) attempt(
	ctx context.Context,
	in <-chan inbound,
	up resource.SRPCResourceService_ResourceRpcClient,
	b *binding,
	again bool,
) (bool, error) {
	// Send again what the client sent on the lost route.
	for _, data := range c.sent {
		if up.Send(newData(data)) != nil {
			return false, nil
		}
	}
	if c.clientDone {
		_ = up.CloseSend()
	}

	// Read the upstream route until this attempt ends.
	upCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	upIn := make(chan inbound)
	go pump(upCtx, up, upIn)

	// A reopened stream's first message repeats what the client holds when
	// its bytes equal the last one forwarded.
	dropRepeat := again && c.rule == Reopen
	for {
		select {
		case <-ctx.Done():
			return true, nil
		case <-b.up.lost:
			return false, nil
		case m := <-in:
			if errors.Is(m.err, io.EOF) {
				c.clientDone = true
				in = nil
				_ = up.CloseSend()
				continue
			}
			if m.err != nil {
				return true, nil
			}
			c.fromClient(up, m.pkt)
		case m := <-upIn:
			if m.err != nil {
				return false, nil
			}
			settled, err := c.fromUpstream(b, m.pkt, &dropRepeat)
			if errors.Is(err, errStale) {
				return false, nil
			}
			if err != nil {
				return true, err
			}
			if settled {
				return true, c.finish(ctx, in, up)
			}
		}
	}
}

// fromClient sends a client packet upstream, keeping it when the call's rule
// may send it again. A failed send shows as the upstream route's end.
func (c *call) fromClient(up resource.SRPCResourceService_ResourceRpcClient, pkt *resource.ResourceRpcPacket) {
	if data := pkt.GetData(); data != nil {
		if c.start == nil {
			var p srpc.Packet
			if p.UnmarshalVT(data) == nil && p.GetCallStart() != nil {
				c.start = p.GetCallStart()
				c.rule = c.g.p.rules.Rule(c.start.GetRpcService(), c.start.GetRpcMethod())
			}
		}
		if c.rule != End && c.rule != Once {
			c.sent = append(c.sent, data)
		}
	}
	_ = up.Send(pkt)
}

// fromUpstream forwards a packet from the route on b to the client and
// reports whether it settled the call.
func (c *call) fromUpstream(b *binding, pkt *resource.ResourceRpcPacket, dropRepeat *bool) (bool, error) {
	// Read the srpc packet the route carries.
	var p srpc.Packet
	var data *srpc.CallData
	if raw := pkt.GetData(); raw != nil && p.UnmarshalVT(raw) == nil {
		data = p.GetCallData()
	}

	// Name an opened child by its proxy ID.
	if msg := data.GetData(); c.rule == Open && len(msg) != 0 {
		msg, ok := c.g.opened(b, c.on, c.replay(), msg)
		if !ok {
			return false, errStale
		}
		data.Data = msg
		raw, err := p.MarshalVT()
		if err != nil {
			return true, err
		}
		pkt = newData(raw)
	}

	// Drop a repeated first message of a reopened stream.
	if msg := data.GetData(); len(msg) != 0 || data.GetDataIsZero() {
		repeat := *dropRepeat && bytes.Equal(msg, c.last)
		*dropRepeat = false
		if repeat && !settles(data) {
			return false, nil
		}
		c.last = msg
	}

	// Forward it.
	if err := c.client.Send(pkt); err != nil {
		return true, err
	}
	c.forwarded = true
	return settles(data), nil
}

// replay returns the call as the client sent it, to send again.
func (c *call) replay() replay {
	return replay{service: c.start.GetRpcService(), method: c.start.GetRpcMethod(), packets: slices.Clone(c.sent)}
}

// finish carries the client's last packets, such as an abandonment, to the
// settled call's upstream until the client closes its route.
func (c *call) finish(ctx context.Context, in <-chan inbound, up resource.SRPCResourceService_ResourceRpcClient) error {
	if c.clientDone {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case m := <-in:
			if m.err != nil {
				_ = up.CloseSend()
				return nil
			}
			_ = up.Send(m.pkt)
		}
	}
}

// resumes reports whether the call opens a new upstream route after its
// upstream was lost.
func (c *call) resumes() bool {
	// A call the client has not started has sent nothing.
	if c.start == nil {
		return true
	}
	switch c.rule {
	case Reopen:
		return !c.clientDone
	case Retry, Open:
		return !c.forwarded
	default:
		return false
	}
}

// reset ends the call with srpc's retryable reset error.
func (c *call) reset() error {
	data, err := srpc.NewCallDataPacket(nil, false, true, srpc.ErrReset).MarshalVT()
	if err != nil {
		return err
	}
	return c.client.Send(newData(data))
}

// settles reports whether data ends its call.
func settles(data *srpc.CallData) bool {
	return data.GetComplete() || data.GetError() != "" || data.GetErrorCode() != srpc.ErrorCode_ERROR_CODE_UNKNOWN
}

// newData wraps an srpc packet in a route packet.
func newData(data []byte) *resource.ResourceRpcPacket {
	return &resource.ResourceRpcPacket{Body: &resource.ResourceRpcPacket_Data{Data: data}}
}

// receiver is a ResourceRpc stream from either side.
type receiver interface {
	Recv() (*resource.ResourceRpcPacket, error)
}

// pump sends each packet r receives to out until r ends or ctx does.
func pump(ctx context.Context, r receiver, out chan<- inbound) {
	for {
		pkt, err := r.Recv()
		select {
		case out <- inbound{pkt: pkt, err: err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}
