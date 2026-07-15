package space_exec

import (
	"context"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_target "github.com/s4wave/spacewave/forge/target"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_vm_world "github.com/s4wave/spacewave/sdk/vm/world"
	"github.com/sirupsen/logrus"
)

// V86ConfigID is the config ID for the V86 VM object handler.
const V86ConfigID = "space-exec/v86"

// v86Config holds the parsed config for the V86 VM object handler.
type v86Config struct {
	// objectKey is the world object key of the vm/v86 object.
	objectKey string
}

type v86InvokerFactory func(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error)

// parseV86Config parses the config from JSON bytes.
// Expected format: {"object_key": "..."}.
func parseV86Config(data []byte) (*v86Config, error) {
	if len(data) == 0 {
		return nil, errors.New("empty config")
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		return nil, errors.Wrap(err, "parse config json")
	}
	objKey := string(v.GetStringBytes("object_key"))
	if objKey == "" {
		return nil, errors.New("object_key is required")
	}
	return &v86Config{objectKey: objKey}, nil
}

// v86Handler starts a vm/v86 object through its PersistentExecutionService.
type v86Handler struct {
	le          *logrus.Entry
	b           bus.Bus
	ws          world.WorldState
	handle      forge_target.ExecControllerHandle
	conf        *v86Config
	loadInvoker v86InvokerFactory
}

// Execute requests desired RUNNING and waits for its process owner to report
// observed RUNNING or a terminal failure.
func (h *v86Handler) Execute(ctx context.Context) error {
	if h.b == nil {
		return errors.New("v86 exec requires bus")
	}
	if h.ws == nil {
		return errors.New("v86 exec requires world state")
	}
	if h.handle == nil {
		return errors.New("v86 exec requires execution handle")
	}

	if err := world_types.CheckObjectType(ctx, h.ws, h.conf.objectKey, s4wave_vm.VmV86TypeID); err != nil {
		return err
	}
	if _, _, err := h.ws.ApplyWorldOp(ctx, s4wave_vm.NewSetV86StateOp(h.conf.objectKey, s4wave_vm.VmState_VmState_STARTING, ""), ""); err != nil {
		return errors.Wrap(err, "set v86 state")
	}

	inv, cleanup, err := h.loadInvoker(ctx, h.le, h.b, h.ws, h.conf.objectKey)
	if err != nil {
		return errors.Wrap(err, "load v86 resource")
	}
	if cleanup != nil {
		defer cleanup()
	}

	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))
	strm, err := s4wave_process.NewSRPCPersistentExecutionServiceClient(client).Execute(ctx, &s4wave_process.ExecuteRequest{})
	if err != nil {
		return errors.Wrap(err, "execute v86 resource")
	}
	defer strm.Close()

	for {
		status, err := strm.Recv()
		if err == io.EOF {
			return errors.New("v86 execution stream ended before RUNNING")
		}
		if err != nil {
			return errors.Wrap(err, "receive v86 execution status")
		}
		if status == nil {
			return errors.New("v86 execution stream returned nil status")
		}
		switch status.GetState() {
		case s4wave_process.ExecutionState_ExecutionState_RUNNING:
			if err := h.handle.WriteLog(ctx, "info", "v86 running: "+h.conf.objectKey); err != nil {
				return err
			}
			return nil
		case s4wave_process.ExecutionState_ExecutionState_ERROR:
			if status.GetError() != "" {
				return errors.New(status.GetError())
			}
			return errors.New("v86 execution failed")
		case s4wave_process.ExecutionState_ExecutionState_STOPPED:
			return errors.New("v86 stopped before RUNNING")
		case s4wave_process.ExecutionState_ExecutionState_STARTING:
			if err := h.handle.WriteLog(ctx, "info", "v86 starting: "+h.conf.objectKey); err != nil {
				return err
			}
		}
	}
}

// NewV86Handler constructs a V86 VM object handler factory.
func NewV86Handler(b bus.Bus) HandlerFactory {
	return newV86Handler(b, defaultV86InvokerFactory)
}

func newV86Handler(b bus.Bus, loadInvoker v86InvokerFactory) HandlerFactory {
	return func(
		ctx context.Context,
		le *logrus.Entry,
		ws world.WorldState,
		handle forge_target.ExecControllerHandle,
		inputs forge_target.InputMap,
		configData []byte,
	) (Handler, error) {
		conf, err := parseV86Config(configData)
		if err != nil {
			return nil, errors.Wrap(err, "parse v86 config")
		}
		return &v86Handler{
			le:          le,
			b:           b,
			ws:          ws,
			handle:      handle,
			conf:        conf,
			loadInvoker: loadInvoker,
		}, nil
	}
}

func defaultV86InvokerFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	return s4wave_vm_world.VmV86Type.GetFactory()(ctx, le, b, nil, ws, objectKey)
}

// RegisterV86 registers the V86 VM object handler in the registry.
func RegisterV86(r *Registry, b bus.Bus) {
	r.Register(V86ConfigID, NewV86Handler(b))
}

// _ is a type assertion.
var _ Handler = (*v86Handler)(nil)
