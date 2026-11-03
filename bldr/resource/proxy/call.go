package resource_proxy

import (
	"bytes"
	"context"
	"io"
	"slices"

	"github.com/aperturerobotics/protobuf-go-lite/encoding/protowire"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
)

// viewField is the field number of the ViewVersion a versioned watch's
// messages carry, and of the held version its request takes.
const viewField = 1000

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
	// unsent holds the packets read before the first upstream route, for a
	// rule that does not send them again.
	unsent []*resource.ResourceRpcPacket
	// put gives back the call's places in the proxy's limits while the call
	// awaits its answer, and is nil otherwise.
	put func()
	// clientDone is set once the client closed its side of the route.
	clientDone bool
	// forwarded is set once an upstream packet reached the client.
	forwarded bool
	// last is the last message forwarded to the client.
	last []byte
	// held is the version of the last message forwarded of a Resume call.
	held string
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

	// Read the call's start, and count a unary call while the proxy holds
	// it; past a limit it ends at once.
	select {
	case <-ctx.Done():
		return nil
	case m := <-in:
		if m.err != nil {
			return nil
		}
		c.keep(m.pkt)
	}
	if c.rule.unary() {
		put, ok := takeAll(c.g.p.limits)
		if !ok {
			return c.reset()
		}
		c.put = put
		defer c.release()
	}

	// Rebind the route until the call completes or its rule ends it.
	for again := false; ; again = true {
		// Open an upstream route, holding the call while there is none.
		up, b, err := c.g.open(ctx, c.on)
		if err != nil {
			if !again || ctx.Err() != nil {
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
	// Send again what the client sent on the lost route, and what it sent
	// before the first.
	for i, data := range c.sent {
		if i == 0 && c.rule == Resume && c.held != "" {
			data = withHeld(data, c.held)
		}
		if up.Send(newData(data)) != nil {
			return false, nil
		}
	}
	for len(c.unsent) != 0 {
		if up.Send(c.unsent[0]) != nil {
			return false, nil
		}
		c.unsent = c.unsent[1:]
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
	// its bytes, or its version, equal the last one forwarded's.
	dropRepeat := again && (c.rule == Reopen || c.rule == Resume)
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
				c.release()
				return true, c.finish(ctx, in, up)
			}
		}
	}
}

// fromClient sends a client packet upstream, keeping it when the call's rule
// may send it again. A failed send shows as the upstream route's end.
func (c *call) fromClient(up resource.SRPCResourceService_ResourceRpcClient, pkt *resource.ResourceRpcPacket) {
	c.record(pkt)
	_ = up.Send(pkt)
}

// keep holds a packet the client sent before the first upstream route.
func (c *call) keep(pkt *resource.ResourceRpcPacket) {
	if !c.record(pkt) {
		c.unsent = append(c.unsent, pkt)
	}
}

// record reads the call's start from a client packet, and keeps the packet
// when the call's rule may send it again. It reports whether it kept it.
func (c *call) record(pkt *resource.ResourceRpcPacket) bool {
	// Read the call method from the first data packet.
	data := pkt.GetData()
	if data == nil {
		return false
	}
	if c.start == nil {
		var p srpc.Packet
		if p.UnmarshalVT(data) == nil && p.GetCallStart() != nil {
			c.start = p.GetCallStart()
			c.rule = c.g.p.rules.Rule(c.start.GetRpcService(), c.start.GetRpcMethod())
		}
	}
	if c.rule == End || c.rule == Once {
		return false
	}
	c.sent = append(c.sent, data)
	return true
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
		var version string
		if c.rule == Resume {
			version = viewVersion(msg)
		}
		repeat := *dropRepeat && (bytes.Equal(msg, c.last) || (version != "" && version == c.held))
		if *dropRepeat && c.rule == Resume && c.g.p.resumed != nil {
			c.g.p.resumed(repeat)
		}
		*dropRepeat = false
		if repeat && !settles(data) {
			return false, nil
		}
		c.last = msg
		if version != "" {
			c.held = version
		}
	}

	// Forward it and keep a successful Replay call.
	if err := c.client.Send(pkt); err != nil {
		return true, err
	}
	c.forwarded = true
	if !settles(data) {
		return false, nil
	}
	ok := data.GetError() == "" && data.GetErrorCode() == srpc.ErrorCode_ERROR_CODE_UNKNOWN
	if ok && c.rule == Replay && c.on == nil {
		c.g.record(c.replay())
	}
	return true, nil
}

// replay returns the call as the client sent it, to send again.
func (c *call) replay() replay {
	return replay{service: c.start.GetRpcService(), method: c.start.GetRpcMethod(), packets: slices.Clone(c.sent)}
}

// release gives back the call's places in the proxy's limits, once.
func (c *call) release() {
	if c.put != nil {
		c.put()
		c.put = nil
	}
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
	case Reopen, Resume:
		return !c.clientDone
	case Retry, Replay, Open:
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

// withHeld returns the srpc packet data with its call's request naming held
// as the version the client holds, or data unchanged when it starts no call.
// The field is appended, so it replaces any version the client sent.
func withHeld(data []byte, held string) []byte {
	// Replace the held version in an encoded call start.
	var p srpc.Packet
	if p.UnmarshalVT(data) != nil || p.GetCallStart() == nil {
		return data
	}
	start := p.GetCallStart()
	req := protowire.AppendTag(slices.Clone(start.GetData()), viewField, protowire.BytesType)
	start.Data, start.DataIsZero = protowire.AppendString(req, held), false
	out, err := p.MarshalVT()
	if err != nil {
		return data
	}
	return out
}

// viewVersion returns the version a versioned watch's message carries, or
// empty when it carries none.
func viewVersion(msg []byte) string {
	view := lastField(msg, viewField)
	return string(lastField(view, 1))
}

// lastField returns the last value of the length-delimited field num in
// msg, or nil when msg holds none or does not parse.
func lastField(msg []byte, num protowire.Number) []byte {
	var out []byte
	for len(msg) != 0 {
		field, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			return nil
		}
		msg = msg[n:]
		if field == num && typ == protowire.BytesType {
			value, m := protowire.ConsumeBytes(msg)
			if m < 0 {
				return nil
			}
			out, msg = value, msg[m:]
			continue
		}
		m := protowire.ConsumeFieldValue(field, typ, msg)
		if m < 0 {
			return nil
		}
		msg = msg[m:]
	}
	return out
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
	// Recv reads the next route packet.
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
