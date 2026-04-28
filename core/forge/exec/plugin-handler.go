package space_exec

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

type pluginExecClientLoader func(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
) (SRPCPluginExecServiceClient, directive.Reference, error)

// pluginExecHandler forwards execution to a plugin-owned controller.
type pluginExecHandler struct {
	b      bus.Bus
	handle forge_target.ExecControllerHandle
	inputs forge_target.InputMap
	conf   *PluginExecConfig
	load   pluginExecClientLoader
}

// Execute loads the target plugin, calls its PluginExecService, and forwards
// logs and outputs back to Forge.
func (h *pluginExecHandler) Execute(ctx context.Context) error {
	client, ref, err := h.load(ctx, h.b, h.conf.GetPluginId())
	if err != nil {
		return errors.Wrap(err, "load plugin exec service")
	}
	if ref != nil {
		defer ref.Release()
	}
	if client == nil {
		return errors.Errorf("plugin not found: %s", h.conf.GetPluginId())
	}

	req := &PluginExecRequest{
		ControllerId:     h.conf.GetControllerId(),
		ControllerConfig: h.conf.GetControllerConfig(),
		Inputs:           h.inputs.BuildValueSet().GetInputs(),
	}
	resp, err := client.Execute(ctx, req)
	if err != nil {
		return errors.Wrap(err, "execute plugin controller")
	}
	if resp == nil {
		return errors.New("plugin exec service returned nil response")
	}
	return h.applyResponse(ctx, resp)
}

func (h *pluginExecHandler) applyResponse(ctx context.Context, resp *PluginExecResponse) error {
	for _, log := range resp.GetLogs() {
		if err := h.handle.WriteLog(ctx, log.GetLevel(), log.GetMessage()); err != nil {
			return err
		}
	}
	if len(resp.GetOutputs()) != 0 {
		if err := h.handle.SetOutputs(ctx, forge_value.ValueSlice(resp.GetOutputs()), true); err != nil {
			return err
		}
	}
	if resp.GetError() != "" {
		return errors.New(resp.GetError())
	}
	return nil
}

func defaultPluginExecClientLoader(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
) (SRPCPluginExecServiceClient, directive.Reference, error) {
	if b == nil {
		return nil, nil, errors.New("plugin exec bridge requires bus")
	}
	client, ref, err := bldr_plugin.ExPluginLoadWaitClient(ctx, b, pluginID, nil)
	if err != nil || client == nil {
		if ref != nil {
			ref.Release()
		}
		return nil, nil, err
	}
	return NewSRPCPluginExecServiceClient(client), ref, nil
}

// NewPluginExecHandler constructs a plugin bridge handler factory.
func NewPluginExecHandler(b bus.Bus) HandlerFactory {
	return newPluginExecHandler(b, defaultPluginExecClientLoader)
}

func newPluginExecHandler(b bus.Bus, load pluginExecClientLoader) HandlerFactory {
	return func(
		ctx context.Context,
		le *logrus.Entry,
		ws world.WorldState,
		handle forge_target.ExecControllerHandle,
		inputs forge_target.InputMap,
		configData []byte,
	) (Handler, error) {
		conf := &PluginExecConfig{}
		if err := conf.UnmarshalVT(configData); err != nil {
			return nil, errors.Wrap(err, "parse plugin exec config")
		}
		if err := conf.Validate(); err != nil {
			return nil, err
		}
		return &pluginExecHandler{
			b:      b,
			handle: handle,
			inputs: inputs,
			conf:   conf,
			load:   load,
		}, nil
	}
}

// RegisterPluginExec registers the plugin bridge handler in the registry.
func RegisterPluginExec(r *Registry, b bus.Bus) {
	r.Register(PluginExecConfigID, NewPluginExecHandler(b))
}

// _ is a type assertion
var _ Handler = (*pluginExecHandler)(nil)
