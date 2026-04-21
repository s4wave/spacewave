//go:build js

package web_runtime_bootstrap

import (
	"context"

	browser "github.com/s4wave/spacewave/bldr/web/entrypoint/browser"
	bldr_web_plugin_browser_controller "github.com/s4wave/spacewave/bldr/web/plugin/browser/controller"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// RuntimeStackOpts configures startup of the browser runtime stack.
type RuntimeStackOpts struct {
	WebRuntimeID   string
	MessagePort    string
	StaticResolver *static.Resolver
}

// RuntimeStack contains the started browser runtime stack.
type RuntimeStack struct {
	WebRuntime web_runtime.WebRuntime
	rels       []func()
}

// Release releases the runtime stack in reverse startup order.
func (s *RuntimeStack) Release() {
	for i := len(s.rels) - 1; i >= 0; i-- {
		if s.rels[i] != nil {
			s.rels[i]()
		}
	}
}

// StartRuntimeStack starts the browser WebRuntime.
func StartRuntimeStack(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	opts RuntimeStackOpts,
) (*RuntimeStack, error) {
	if opts.StaticResolver != nil {
		opts.StaticResolver.AddFactory(browser.NewFactory(b))
	}

	webRuntimeCtrli, _, webRuntimeRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&browser.Config{
			WebRuntimeId: opts.WebRuntimeID,
			MessagePort:  opts.MessagePort,
		}),
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "start web runtime controller")
	}

	webRuntimeCtrl, ok := webRuntimeCtrli.(web_runtime.WebRuntimeController)
	if !ok {
		webRuntimeRef.Release()
		return nil, errors.New("web runtime controller does not implement WebRuntimeController")
	}
	rt, err := webRuntimeCtrl.GetWebRuntime(ctx)
	if err != nil {
		webRuntimeRef.Release()
		return nil, errors.Wrap(err, "get web runtime")
	}

	stack := &RuntimeStack{
		WebRuntime: rt,
		rels:       []func(){webRuntimeRef.Release},
	}

	return stack, nil
}

// StartPluginBrowserHost starts the browser plugin host controller.
func StartPluginBrowserHost(
	ctx context.Context,
	b bus.Bus,
	sr *static.Resolver,
) (func(), error) {
	if sr != nil {
		sr.AddFactory(bldr_web_plugin_browser_controller.NewFactory(b))
	}

	_, _, webPluginBrowserHostRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&bldr_web_plugin_browser_controller.Config{}),
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "start web plugin browser host controller")
	}
	return webPluginBrowserHostRef.Release, nil
}
