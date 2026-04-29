package space_exec

import (
	"bytes"
	"context"
	"path"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	bifrost_rpc_access "github.com/s4wave/spacewave/net/rpc/access"
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
	outputs := forge_value.ValueSlice(resp.GetOutputs()).Clone()
	if len(resp.GetOutputFiles()) != 0 {
		fileOutputs, err := h.importOutputFiles(ctx, resp.GetOutputFiles())
		if err != nil {
			return errors.Wrap(err, "import plugin output files")
		}
		outputs = append(outputs, fileOutputs...)
	}
	if len(outputs) != 0 {
		if err := h.handle.SetOutputs(ctx, outputs, true); err != nil {
			return err
		}
	}
	if resp.GetError() != "" {
		return errors.New(resp.GetError())
	}
	return nil
}

func (h *pluginExecHandler) importOutputFiles(
	ctx context.Context,
	files []*PluginExecOutputFile,
) (forge_value.ValueSlice, error) {
	var outputs forge_value.ValueSlice
	err := h.handle.AccessStorage(ctx, nil, func(cs *bucket_lookup.Cursor) error {
		outputHandle, err := initPluginOutputMount(ctx, cs)
		if err != nil {
			return err
		}
		defer outputHandle.Release()

		ts := time.Now()
		for _, file := range files {
			parts, err := cleanOutputFilePath(file.GetPath())
			if err != nil {
				return err
			}
			dir := outputHandle
			if len(parts) > 1 {
				dir, err = outputHandle.MkdirAllLookup(ctx, parts[:len(parts)-1], 0o755, ts)
				if err != nil {
					return err
				}
				defer dir.Release()
			}
			if err := dir.MknodWithContent(
				ctx,
				parts[len(parts)-1],
				unixfs.NewFSCursorNodeType_File(),
				int64(len(file.GetData())),
				bytes.NewReader(file.GetData()),
				0o644,
				ts,
			); err != nil {
				return err
			}
		}

		outputRef := cs.GetRefWithOpArgs()
		if outputRef == nil || outputRef.GetRootRef().GetEmpty() {
			return nil
		}
		outputs = forge_value.ValueSlice{
			forge_value.NewValueWithBucketRef("output", outputRef),
		}
		return nil
	})
	return outputs, err
}

func cleanOutputFilePath(filePath string) ([]string, error) {
	cleaned := path.Clean(strings.TrimPrefix(filePath, "/"))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return nil, errors.Errorf("invalid output file path: %s", filePath)
	}
	parts := strings.Split(cleaned, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.Errorf("invalid output file path: %s", filePath)
		}
	}
	return parts, nil
}

func initPluginOutputMount(ctx context.Context, cs *bucket_lookup.Cursor) (*unixfs.FSHandle, error) {
	btx, bcs := cs.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)
	if _, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY); err != nil {
		return nil, errors.Wrap(err, "create root fstree")
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "write root block")
	}
	cs.SetRootRef(rootRef)

	wr := unixfs_block_fs.NewFSWriter()
	fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cs, wr)
	wr.SetFS(fs)

	handle, err := unixfs.NewFSHandle(fs)
	if err != nil {
		fs.Release()
		return nil, errors.Wrap(err, "create fshandle")
	}
	return handle, nil
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
	accessClient := bifrost_rpc_access.NewSRPCAccessRpcServiceClient(client)
	req := bifrost_rpc_access.NewLookupRpcServiceRequest(SRPCPluginExecServiceServiceID, "")
	invoker := bifrost_rpc_access.NewProxyInvoker(accessClient, req, true)
	proxyClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	return NewSRPCPluginExecServiceClient(proxyClient), ref, nil
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
