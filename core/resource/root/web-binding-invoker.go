//go:build !tinygo

package resource_root

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// webBindingInvoker serves only the methods a webBinding admits.
type webBindingInvoker struct {
	// binding identifies the scope checked by each message adapter.
	binding *webBinding
	// inv dispatches admitted methods to the Resource.
	inv srpc.Invoker
}

// InvokeMethod denies methods outside webBindingMethods, checks the request
// messages of the methods that need checks, and narrows their responses.
func (i *webBindingInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Deny methods outside the list, then route the call through its check
	// and response filter.
	key := webMethod(serviceID, methodID)
	check, ok := webBindingMethods[key]
	if !ok {
		i.binding.le.WithField("method", key).Info("web binding denied method")
		return true, errors.Wrap(errWebBindingDenied, key)
	}

	// Wrap messages only when the method needs validation or projection.
	narrow := webBindingResponses[key]
	if check != nil || narrow != nil {
		strm = &webBindingStream{
			Stream:  strm,
			binding: i.binding,
			method:  key,
			check:   check,
			narrow:  narrow,
		}
	}

	// Dispatch through the selected message adapter.
	return i.inv.InvokeMethod(serviceID, methodID, strm)
}

// newWebBindingFilter wraps each Resource invoker in the bound listener's policy.
func newWebBindingFilter(binding *webBinding) resource_server.InvokerFilter {
	return func(inv srpc.Invoker) srpc.Invoker {
		return &webBindingInvoker{binding: binding, inv: inv}
	}
}

var _ srpc.Invoker = (*webBindingInvoker)(nil)
