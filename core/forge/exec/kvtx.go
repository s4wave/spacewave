package space_exec

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"

	// Blank import to vendor the kvtx package.
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
)

// KvtxConfigID is the config ID for the space-aware kvtx handler.
// Matches the existing forge/lib/kvtx ConfigID so existing task targets work.
var KvtxConfigID = forge_lib_kvtx.ConfigID

// kvtxHandler wraps the existing kvtx controller without bus access.
type kvtxHandler struct {
	ctrl *forge_lib_kvtx.Controller
}

// Execute runs the kvtx handler.
func (h *kvtxHandler) Execute(ctx context.Context) error {
	return h.ctrl.Execute(ctx)
}

// NewKvtxHandler constructs a kvtx space handler.
// Deserializes configData as the kvtx Config proto, constructs the controller
// with a nil bus (unused by kvtx), and initializes it with inputs and handle.
func NewKvtxHandler(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	handle forge_target.ExecControllerHandle,
	inputs forge_target.InputMap,
	configData []byte,
) (Handler, error) {
	conf := &forge_lib_kvtx.Config{}
	if len(configData) > 0 {
		if err := conf.UnmarshalVT(configData); err != nil {
			return nil, errors.Wrap(err, "unmarshal kvtx config")
		}
	}
	if err := conf.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate kvtx config")
	}

	ctrl := forge_lib_kvtx.NewController(le, nil, conf)
	if err := ctrl.InitForgeExecController(ctx, inputs, handle); err != nil {
		return nil, errors.Wrap(err, "init kvtx controller")
	}

	return &kvtxHandler{ctrl: ctrl}, nil
}

// RegisterKvtx registers the kvtx handler in the registry.
func RegisterKvtx(r *Registry) {
	r.Register(KvtxConfigID, NewKvtxHandler)
}

// _ is a type assertion
var _ Handler = (*kvtxHandler)(nil)
