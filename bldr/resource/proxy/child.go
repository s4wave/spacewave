package resource_proxy

import (
	"slices"

	"github.com/s4wave/spacewave/bldr/resource"
	"google.golang.org/protobuf/encoding/protowire"
)

// childIDField is the response field an Open method carries its child's ID
// in, as every Resource SDK opener does.
const childIDField = 1

// child is a resource an Open call added beneath a generation's root or
// beneath another child. Its proxy ID stays fixed while its upstream ID
// changes with each binding. Its fields are guarded by its generation's bcast.
type child struct {
	// g is the generation the child belongs to.
	g *generation
	// id is the child ID the proxy issued.
	id uint32
	// parent is the child the Open call ran on, or nil for the root.
	parent *child
	// call is the Open call, sent again on each new upstream.
	call replay

	// up is the child's ID on the binding on.
	up uint32
	// on is the binding up belongs to.
	on *binding
	// adopted is set once the client adopted the child, so the proxy adopts
	// it again on each new upstream.
	adopted bool
	// released is set once the child is gone from the client's view.
	released bool
}

// errReleased ends a call on a child the proxy has released.
var errReleased = resource.ErrResourceOrClientReleased

// opened registers the child an Open call answered on b with msg, and returns
// msg naming the child by its proxy ID. It reports false when b no longer
// carries the generation: that upstream is lost, so the call opens again on
// the next.
func (g *generation) opened(b *binding, parent *child, call replay, msg []byte) ([]byte, bool) {
	up, ok := readChildID(msg)
	if !ok {
		return msg, true
	}
	var ch *child
	g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if g.bound != b {
			return
		}
		ch = &child{g: g, parent: parent, call: call, up: up, on: b}
		g.p.addChild(ch)
		g.children = append(g.children, ch)
	})
	if ch == nil {
		return nil, false
	}
	return writeChildID(msg, ch.id), true
}

// childLocked returns the live child with proxy ID id, or nil. The caller
// holds bcast.
func (g *generation) childLocked(id uint32) *child {
	for _, ch := range g.children {
		if ch.id == id {
			return ch
		}
	}
	return nil
}

// dropLocked removes ch from the generation without telling the client. The
// caller holds bcast.
func (g *generation) dropLocked(ch *child) {
	ch.released = true
	g.children = slices.DeleteFunc(g.children, func(c *child) bool { return c == ch })
	g.p.dropChildren(ch.id)
}

// releaseLocked removes ch and tells the client it is released. The caller
// holds bcast.
func (g *generation) releaseLocked(ch *child) error {
	g.dropLocked(ch)
	return g.sendReleasedLocked(ch.id)
}

// sendReleasedLocked tells the client the resource id is released. The caller
// holds bcast.
func (g *generation) sendReleasedLocked(id uint32) error {
	return g.client.Send(&resource.ResourceClientResponse{Body: &resource.ResourceClientResponse_ResourceReleased{
		ResourceReleased: &resource.ResourceReleasedResponse{ResourceId: id},
	}})
}

// childIDs returns the proxy IDs of the live children.
func (g *generation) childIDs() []uint32 {
	var ids []uint32
	g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, ch := range g.children {
			ids = append(ids, ch.id)
		}
	})
	return ids
}

// readChildID returns the child ID in an Open response, or false when it
// names none.
func readChildID(msg []byte) (uint32, bool) {
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			return 0, false
		}
		msg = msg[n:]
		if num == childIDField && typ == protowire.VarintType {
			v, n := protowire.ConsumeVarint(msg)
			return uint32(v), n > 0 && v != 0
		}
		n = protowire.ConsumeFieldValue(num, typ, msg)
		if n < 0 {
			return 0, false
		}
		msg = msg[n:]
	}
	return 0, false
}

// writeChildID returns an Open response with its child ID replaced by id.
func writeChildID(msg []byte, id uint32) []byte {
	out := protowire.AppendTag(nil, childIDField, protowire.VarintType)
	out = protowire.AppendVarint(out, uint64(id))
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			return append(out, msg...)
		}
		v := protowire.ConsumeFieldValue(num, typ, msg[n:])
		if v < 0 {
			return append(out, msg...)
		}
		m := n + v
		if num != childIDField {
			out = append(out, msg[:m]...)
		}
		msg = msg[m:]
	}
	return out
}
