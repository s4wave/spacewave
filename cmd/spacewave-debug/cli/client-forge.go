//go:build !js

package cli

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"strconv"

	appcli "github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	forge_value "github.com/s4wave/spacewave/forge/value"
	bifrost_rpc_access "github.com/s4wave/spacewave/net/rpc/access"
)

// ForgeArgs contains forge debug command arguments.
type ForgeArgs struct {
	client *ClientArgs

	// TargetPath is a forge/target/json YAML or JSON target file.
	TargetPath string
	// DryRun validates the target without opening the debug bridge.
	DryRun bool
}

// BuildForgeCommand returns debug commands for live Forge execution probes.
func (a *ClientArgs) BuildForgeCommand() *appcli.Command {
	fa := &ForgeArgs{client: a}
	return &appcli.Command{
		Name:  "forge",
		Usage: "run live Forge debug probes through the app debug bridge",
		Subcommands: []*appcli.Command{
			{
				Name:  "run-plugin-target",
				Usage: "run a space-exec/plugin target through the live app plugin host",
				Flags: []appcli.Flag{
					&appcli.StringFlag{
						Name:        "target",
						Aliases:     []string{"f"},
						Usage:       "path to a forge/target/json YAML or JSON target",
						Required:    true,
						Destination: &fa.TargetPath,
					},
					&appcli.BoolFlag{
						Name:        "dry-run",
						Usage:       "validate and print the target route without executing it",
						Destination: &fa.DryRun,
					},
				},
				Action: fa.RunPluginTarget,
			},
		},
	}
}

// RunPluginTarget runs a space-exec/plugin target through the live debug bridge.
func (fa *ForgeArgs) RunPluginTarget(c *appcli.Context) error {
	ctx := c.Context
	data, err := os.ReadFile(fa.TargetPath)
	if err != nil {
		return errors.Wrapf(err, "read target %s", fa.TargetPath)
	}
	conf, err := parsePluginExecTarget(data)
	if err != nil {
		return err
	}
	w := os.Stdout
	w.WriteString("plugin: " + conf.GetPluginId() + "\n")
	w.WriteString("controller: " + conf.GetControllerId() + "\n")
	w.WriteString("controller-config-bytes: " + strconv.Itoa(len(conf.GetControllerConfig())) + "\n")
	if fa.DryRun {
		return nil
	}
	client, err := fa.client.BuildPluginExecServiceClient(ctx, conf.GetPluginId())
	if err != nil {
		return err
	}
	req := &space_exec.PluginExecRequest{
		ControllerId:     conf.GetControllerId(),
		ControllerConfig: conf.GetControllerConfig(),
	}
	strm, err := client.ExecuteStream(ctx, req)
	if err == nil {
		defer strm.Close()
		return printPluginExecStream(ctx, w, strm)
	}
	resp, err := client.Execute(ctx, req)
	if err != nil {
		return errors.Wrap(err, "execute plugin target")
	}
	return printPluginExecResponse(ctx, w, resp)
}

// BuildPluginExecServiceClient returns the PluginExecService for a live plugin.
func (a *ClientArgs) BuildPluginExecServiceClient(
	ctx context.Context,
	pluginID string,
) (space_exec.SRPCPluginExecServiceClient, error) {
	pluginClient, err := a.DialPluginRpc(pluginID)
	if err != nil {
		return nil, err
	}
	accessClient := bifrost_rpc_access.NewSRPCAccessRpcServiceClient(pluginClient)
	req := bifrost_rpc_access.NewLookupRpcServiceRequest(space_exec.SRPCPluginExecServiceServiceID, "")
	invoker := bifrost_rpc_access.NewProxyInvoker(accessClient, req, true)
	proxyClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	return space_exec.NewSRPCPluginExecServiceClient(proxyClient), nil
}

func parsePluginExecTarget(data []byte) (*space_exec.PluginExecConfig, error) {
	jdata, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, errors.Wrap(err, "parse target yaml")
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(jdata)
	if err != nil {
		return nil, errors.Wrap(err, "parse target json")
	}
	if inputs := v.GetArray("inputs"); len(inputs) != 0 {
		return nil, errors.New("debug plugin target runner does not support target inputs yet")
	}
	controller := v.Get("exec", "controller")
	if controller == nil || controller.Type() != fastjson.TypeObject {
		return nil, errors.New("target exec.controller is required")
	}
	if id := stringValue(controller.Get("id")); id != space_exec.PluginExecConfigID {
		return nil, errors.Errorf("target exec.controller.id must be %s, got %q", space_exec.PluginExecConfigID, id)
	}
	config := controller.Get("config")
	if config == nil || config.Type() != fastjson.TypeObject {
		return nil, errors.New("target exec.controller.config is required")
	}
	controllerConfig, err := base64.StdEncoding.DecodeString(stringValue(config.Get("controllerConfig")))
	if err != nil {
		return nil, errors.Wrap(err, "decode controllerConfig")
	}
	conf := &space_exec.PluginExecConfig{
		PluginId:         stringValue(config.Get("pluginId")),
		ControllerId:     stringValue(config.Get("controllerId")),
		ControllerConfig: controllerConfig,
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return conf, nil
}

func stringValue(v *fastjson.Value) string {
	if v == nil {
		return ""
	}
	return string(v.GetStringBytes())
}

func printPluginExecStream(
	ctx context.Context,
	w io.Writer,
	strm space_exec.SRPCPluginExecService_ExecuteStreamClient,
) error {
	for {
		resp, err := strm.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "receive plugin exec stream")
		}
		if err := printPluginExecResponse(ctx, w, resp); err != nil {
			return err
		}
	}
}

func printPluginExecResponse(ctx context.Context, w io.Writer, resp *space_exec.PluginExecResponse) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if resp == nil {
		return errors.New("plugin exec service returned nil response")
	}
	for _, entry := range resp.GetLogs() {
		if _, err := io.WriteString(w, "log["+entry.GetLevel()+"]: "+entry.GetMessage()+"\n"); err != nil {
			return err
		}
	}
	if err := printPluginExecOutputs(w, resp.GetOutputs()); err != nil {
		return err
	}
	for _, file := range resp.GetOutputFiles() {
		if _, err := io.WriteString(w, "output-file: "+file.GetPath()+" bytes="+strconv.Itoa(len(file.GetData()))+"\n"); err != nil {
			return err
		}
	}
	if resp.GetError() != "" {
		return errors.New(resp.GetError())
	}
	return nil
}

func printPluginExecOutputs(w io.Writer, outputs []*forge_value.Value) error {
	for _, output := range outputs {
		if output == nil {
			continue
		}
		if _, err := io.WriteString(w, "output: "+output.GetName()+"\n"); err != nil {
			return err
		}
	}
	return nil
}
