package space_exec

import (
	"bytes"
	"context"
	"io"
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

// pluginExecClientLoader retains the plugin service and reports its load wait.
type pluginExecClientLoader func(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
	onWaiting func() error,
) (SRPCPluginExecServiceClient, directive.Reference, error)

// pluginExecHandler forwards execution to a plugin-owned controller.
type pluginExecHandler struct {
	// b resolves the plugin runtime and World ObjectType services.
	b bus.Bus
	// le records execution diagnostics.
	le *logrus.Entry
	// handle retains the caller's Execution identity and granted claim epoch.
	handle forge_target.ExecControllerHandle
	// inputs are the resolved inputs from the Execution snapshot.
	inputs forge_target.InputMap
	// conf selects the plugin controller and its World grant.
	conf *PluginExecConfig
	// load retains the plugin service for Execute's lifetime.
	load pluginExecClientLoader
}

// Execute loads the target plugin, calls its PluginExecService, and forwards
// logs and outputs back to Forge.
func (h *pluginExecHandler) Execute(ctx context.Context) error {
	// Retain the plugin service while recording any load wait on the Execution.
	var waited bool
	client, ref, err := h.load(ctx, h.b, h.conf.GetPluginId(), func() error {
		waited = true
		return h.handle.SetWaitingPlugin(ctx, h.conf.GetPluginId())
	})
	if err != nil {
		return errors.Wrap(err, "load plugin exec service")
	}
	if ref != nil {
		defer ref.Release()
	}
	if client == nil {
		return errors.Errorf("plugin not found: %s", h.conf.GetPluginId())
	}
	if waited {
		if err := h.handle.SetWaitingPlugin(ctx, ""); err != nil {
			return errors.Wrap(err, "clear plugin load wait")
		}
	}

	// Forward the granted Execution context alongside the controller's inputs.
	req := &PluginExecRequest{
		ControllerId:       h.conf.GetControllerId(),
		ControllerConfig:   h.conf.GetControllerConfig(),
		Inputs:             h.inputs.BuildValueSet().GetInputs(),
		ExecutionObjectKey: h.handle.GetExecutionObjectKey(),
		ClaimEpoch:         h.handle.GetExecutionClaimEpoch(),
	}
	if h.conf.GetAttachWorld() {
		return h.executeWithWorld(ctx, client, req)
	}
	return h.executeClient(ctx, client, req)
}

// executeClient relays plugin progress and joins the execution stream.
func (h *pluginExecHandler) executeClient(ctx context.Context, client SRPCPluginExecServiceClient, req *PluginExecRequest) error {
	// Join the plugin's streamed execution when the service supports it.
	strm, err := client.ExecuteStream(ctx, req)
	if err == nil {
		defer strm.Close()
		return h.applyStream(ctx, strm)
	}

	// Invoke the unary service when a stream could not be opened.
	resp, err := client.Execute(ctx, req)
	if err != nil {
		return errors.Wrap(err, "execute plugin controller")
	}
	if resp == nil {
		return errors.New("plugin exec service returned nil response")
	}
	return h.applyResponse(ctx, resp)
}

// applyStream applies every plugin response until the execution stream ends.
func (h *pluginExecHandler) applyStream(
	ctx context.Context,
	strm SRPCPluginExecService_ExecuteStreamClient,
) error {
	received := false
	for {
		// Receive the next plugin response or terminal stream result.
		resp, err := strm.Recv()
		if err == io.EOF {
			if !received {
				return errors.New("plugin exec stream completed without a response")
			}
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "receive plugin execution stream")
		}

		// Persist plugin progress before accepting the next stream response.
		if resp == nil {
			return errors.New("plugin exec service returned nil stream response")
		}
		if err := h.applyResponse(ctx, resp); err != nil {
			return err
		}
		received = true
	}
}

// applyResponse persists the plugin's logs and outputs through the granted handle.
func (h *pluginExecHandler) applyResponse(ctx context.Context, resp *PluginExecResponse) error {
	// Append the plugin logs to the Execution.
	for _, log := range resp.GetLogs() {
		if err := h.handle.WriteLog(ctx, log.GetLevel(), log.GetMessage()); err != nil {
			return err
		}
	}

	// Import output files and replace the Execution's outputs when supplied.
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

	// Propagate the plugin's reported execution failure.
	if resp.GetError() != "" {
		return errors.New(resp.GetError())
	}
	return nil
}

// importOutputFiles stores plugin output files in a retained UnixFS output mount.
func (h *pluginExecHandler) importOutputFiles(
	ctx context.Context,
	files []*PluginExecOutputFile,
) (forge_value.ValueSlice, error) {
	var outputs forge_value.ValueSlice
	err := h.handle.AccessStorage(ctx, nil, func(cs *bucket_lookup.Cursor) error {
		// Create the UnixFS mount within the granted storage cursor.
		outputHandle, err := initPluginOutputMount(ctx, cs)
		if err != nil {
			return err
		}
		defer outputHandle.Release()

		// Write each plugin output file below the mount root.
		ts := time.Now()
		for _, file := range files {
			// Resolve the plugin file's parent directory within the output mount.
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

			// Write the plugin file content into its mount directory.
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

		// Return the mount's immutable reference as the output value.
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

// cleanOutputFilePath confines a plugin output path to its UnixFS mount.
func cleanOutputFilePath(filePath string) ([]string, error) {
	// Normalize the slash path within the plugin output mount.
	cleaned := path.Clean(strings.TrimPrefix(filePath, "/"))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return nil, errors.Errorf("invalid output file path: %s", filePath)
	}

	// Require each output path component to name a mount child.
	parts := strings.Split(cleaned, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.Errorf("invalid output file path: %s", filePath)
		}
	}
	return parts, nil
}

// initPluginOutputMount initializes a directory root in the supplied storage cursor.
func initPluginOutputMount(ctx context.Context, cs *bucket_lookup.Cursor) (*unixfs.FSHandle, error) {
	// Write the UnixFS directory root and tree into the granted storage cursor.
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

	// Bind a writable UnixFS view to the retained root.
	wr := unixfs_block_fs.NewFSWriter()
	fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cs, wr)
	wr.SetFS(fs)

	// Transfer the filesystem view to the returned handle.
	handle, err := unixfs.NewFSHandle(fs)
	if err != nil {
		fs.Release()
		return nil, errors.Wrap(err, "create fshandle")
	}
	return handle, nil
}

// defaultPluginExecClientLoader waits on plugin demand and retains its RPC route.
func defaultPluginExecClientLoader(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
	onWaiting func() error,
) (SRPCPluginExecServiceClient, directive.Reference, error) {
	// Require the bus that resolves plugin runtime demand.
	if b == nil {
		return nil, nil, errors.New("plugin exec bridge requires bus")
	}

	// Use the loaded plugin or wait for its client through the plugin directive.
	running, _, initialRef, err := bldr_plugin.ExLoadPlugin(ctx, b, true, pluginID, nil)
	if err != nil {
		return nil, nil, err
	}
	var client srpc.Client
	var ref directive.Reference
	if running != nil {
		client = running.GetRpcClient()
		ref = initialRef
	}
	if running == nil {
		if initialRef != nil {
			initialRef.Release()
		}
		if err := onWaiting(); err != nil {
			return nil, nil, err
		}
		client, ref, err = bldr_plugin.ExPluginLoadWaitClient(ctx, b, pluginID, nil)
	}
	if err != nil || client == nil {
		if ref != nil {
			ref.Release()
		}
		return nil, nil, err
	}

	// Forward the execution service over the retained plugin RPC route.
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

// newPluginExecHandler binds the plugin client loader to validated execution config.
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
			le:     le,
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

// _ verifies the execution handler contract.
var _ Handler = (*pluginExecHandler)(nil)
