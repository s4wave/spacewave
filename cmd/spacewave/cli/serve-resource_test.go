//go:build !js

package spacewave_cli

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// TestLookupLocalResourceInvokerSelectsRegisteredService pins the CLI path to
// the in-process Resource service and retains its directive reference.
func TestLookupLocalResourceInvokerSelectsRegisteredService(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(nil).Register(mux); err != nil {
		t.Fatal(err)
	}
	ctrl := bifrost_rpc.NewInvokerController(
		le,
		b,
		controller.NewInfo("test/local-resource", controller.MustParseVersion("0.0.1"), ""),
		mux,
		[]string{resource.SRPCResourceServiceServiceID},
	)
	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	invoker, invokerRef, err := lookupLocalResourceInvoker(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if invoker == nil || invokerRef == nil {
		t.Fatal("local Resource lookup returned an incomplete result")
	}
	invokerRef.Release()
}
