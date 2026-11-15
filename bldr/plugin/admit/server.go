package bldr_plugin_admit

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// ViaBusControllerID is the controller ID of the PluginRootAdmission server.
const ViaBusControllerID = "bldr/plugin/admit/via-bus"

// ViaBus serves PluginRootAdmission from the AdmitPluginRoot directive on the
// owner's bus, where the approvals controller watches its own store.
type ViaBus struct {
	b bus.Bus
}

// NewViaBus constructs a PluginRootAdmission server over b.
func NewViaBus(b bus.Bus) *ViaBus {
	return &ViaBus{b: b}
}

// NewViaBusController constructs a controller serving PluginRootAdmission on
// b's LookupRpcService, so the plugin host reaches it through the plugin.
func NewViaBusController(le *logrus.Entry, b bus.Bus) *bifrost_rpc.InvokerController {
	mux := srpc.NewMux()
	_ = SRPCRegisterPluginRootAdmission(mux, NewViaBus(b))
	return bifrost_rpc.NewInvokerController(
		le,
		b,
		controller.NewInfo(ViaBusControllerID, Version, "PluginRootAdmission rpc to directive"),
		mux,
		nil,
	)
}

// WatchPluginRootAdmitted streams the root's answer: the first once the
// directive holds a value or goes idle, then each change.
func (v *ViaBus) WatchPluginRootAdmitted(
	req *WatchPluginRootAdmittedRequest,
	strm SRPCPluginRootAdmission_WatchPluginRootAdmittedStream,
) error {
	// Build the directive for the requested root.
	if req.GetPeerId() == "" {
		return errors.New("peer id cannot be empty")
	}
	dir := bldr_plugin.NewAdmitPluginRoot(req.GetPeerId(), req.GetPluginId(), req.GetManifestRoot())
	if err := dir.Validate(); err != nil {
		return err
	}

	// Track the value count and idle state under one broadcast.
	var bcast broadcast.Broadcast
	var vals int
	var idle bool
	update := func(fn func()) {
		l := bcast.Lock()
		fn()
		l.Broadcast()
		l.Unlock()
	}

	// Watch the directive on the bus for as long as the caller stays.
	ctx := strm.Context()
	di, ref, err := v.b.AddDirective(ctx, dir, bus.NewCallbackHandler(
		func(directive.AttachedValue) { update(func() { vals++ }) },
		func(directive.AttachedValue) { update(func() { vals-- }) },
		nil,
	))
	if err != nil {
		return err
	}
	defer ref.Release()
	defer di.AddIdleCallback(func(isIdle bool, _ []error) {
		update(func() { idle = isIdle })
	})()

	// Send each answered change until the caller goes away.
	var sent, last bool
	for {
		l := bcast.Lock()
		admitted, answered := vals != 0, vals != 0 || idle
		waitCh := l.WaitCh()
		l.Unlock()
		if answered && (!sent || admitted != last) {
			if err := strm.Send(&WatchPluginRootAdmittedResponse{Admitted: admitted}); err != nil {
				return err
			}
			sent, last = true, admitted
		}
		select {
		case <-ctx.Done():
			return nil
		case <-waitCh:
		}
	}
}

// _ is a type assertion
var _ SRPCPluginRootAdmissionServer = (*ViaBus)(nil)
