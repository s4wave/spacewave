package resource_client

import (
	"context"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// AttachedResourceInvoker serves a client-published resource tree. Handlers publish
// children before sending the response containing their IDs. Children
// remain owned by their creating invocation until a response is sent successfully;
// unsent children are detached when the invocation ends. Sent children retain
// the client's attachment lifetime, including after a streaming call ends.
type AttachedResourceInvoker struct {
	// client publishes and detaches the tree's children.
	client *Client
	// mux implements the attached resource's methods.
	mux srpc.Invoker
}

// NewAttachedResourceInvoker wraps a mux with invocation-scoped child publication.
func NewAttachedResourceInvoker(client *Client, mux srpc.Invoker) *AttachedResourceInvoker {
	return &AttachedResourceInvoker{client: client, mux: mux}
}

// InvokeMethod releases every child whose publication did not reach a response.
func (i *AttachedResourceInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Scope pending attachments to this invocation while preserving child lifetimes.
	ctx, cancel := context.WithCancel(strm.Context())
	defer cancel()
	invocation := &attachedResourceInvocation{client: i.client, ctx: ctx}
	defer invocation.releasePending()

	// Serve the method with a publication context and a response transfer boundary.
	ctx = resource_server.WithResourceClientContext(ctx, invocation)
	return i.mux.InvokeMethod(serviceID, methodID, &attachedResourceStream{
		Stream:     srpc.NewStreamWithContext(strm, ctx),
		invocation: invocation,
	})
}

// attachedResourceInvocation retains the children not yet transferred by a response.
type attachedResourceInvocation struct {
	// client owns the shared attachment lifetime.
	client *Client
	// ctx scopes attachment negotiation to the creating invocation.
	ctx context.Context
	// mtx serializes publication, response transfer, and invocation completion.
	mtx sync.Mutex
	// pending maps unsent children to their publishing session.
	pending map[uint32]*attachSession
	// ended rejects publication after invocation completion.
	ended bool
}

// Context returns the attachment lifetime used by independently retained children.
func (i *attachedResourceInvocation) Context() context.Context { return i.client.attachCtx }

// AddResource publishes an invocation child with a release callback.
func (i *attachedResourceInvocation) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return i.AddResourceValue(mux, nil, releaseFn)
}

// AddResourceValue publishes a child's mux without exposing its in-process value.
func (i *attachedResourceInvocation) AddResourceValue(mux srpc.Invoker, _ any, releaseFn func()) (uint32, error) {
	// Serialize child negotiation with response sends and invocation completion.
	i.mtx.Lock()
	defer i.mtx.Unlock()
	if i.ended {
		return 0, context.Canceled
	}

	// Retain the successfully published child until a response transfers it.
	id, sess, err := i.client.attachResource(i.ctx, "attached-child", mux)
	if err != nil {
		return 0, err
	}
	sess.setRelease(id, releaseFn)
	if i.pending == nil {
		i.pending = make(map[uint32]*attachSession)
	}
	i.pending[id] = sess
	return id, nil
}

// ReleaseResource withdraws a child before or after response transfer.
func (i *attachedResourceInvocation) ReleaseResource(id uint32) bool {
	// Preserve invocation cleanup if the attachment transport refuses the detach.
	if err := i.client.DetachResource(i.client.attachCtx, id); err != nil {
		return false
	}

	// Remove the detached child from pending response ownership.
	i.mtx.Lock()
	delete(i.pending, id)
	i.mtx.Unlock()
	return true
}

// GetResourceValue rejects values that belong to the remote attachment endpoint.
func (i *attachedResourceInvocation) GetResourceValue(uint32) (any, error) {
	return nil, resource.ErrResourceNotFound
}

// GetAttachedResource rejects raw clients that belong to the remote endpoint.
func (i *attachedResourceInvocation) GetAttachedResource(uint32) (srpc.Client, error) {
	return nil, resource.ErrResourceNotFound
}

// releasePending ends publication and releases children absent from sent responses.
func (i *attachedResourceInvocation) releasePending() {
	// Retire the invocation and take its remaining children under the publication lock.
	i.mtx.Lock()
	i.ended = true
	pending := i.pending
	i.pending = nil
	i.mtx.Unlock()

	// Release local handles even if the attachment transport cannot send a detach.
	for id, sess := range pending {
		sess.releaseAttachedResource(id)
		_ = sess.sendDetach(id)
	}
}

// attachedResourceStream transfers pending children only after a successful send.
type attachedResourceStream struct {
	// Stream carries the invocation's messages and context.
	srpc.Stream
	// invocation owns the pending response's children.
	invocation *attachedResourceInvocation
}

// MsgSend preserves pending ownership when cancellation or transport failure prevents sending.
func (s *attachedResourceStream) MsgSend(msg srpc.Message) error {
	// Keep child publication stable while sending the response that transfers it.
	s.invocation.mtx.Lock()
	defer s.invocation.mtx.Unlock()
	if err := s.Context().Err(); err != nil {
		return err
	}

	// Transfer pending children only after the transport accepts the response.
	if err := s.Stream.MsgSend(msg); err != nil {
		return err
	}
	s.invocation.pending = nil
	return nil
}

// _ asserts the attached invocation and stream contracts.
var (
	_ srpc.Invoker                          = (*AttachedResourceInvoker)(nil)
	_ resource_server.ResourceClientContext = (*attachedResourceInvocation)(nil)
	_ srpc.Stream                           = (*attachedResourceStream)(nil)
)
