package hydra_api

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"storj.io/drpc"
)

// API implements the DRPC API.
type API struct {
	bus  bus.Bus
	conf *Config
}

// NewAPI constructs a new instance of the API.
func NewAPI(bus bus.Bus, conf *Config) (*API, error) {
	return &API{bus: bus, conf: conf}, nil
}

// RegisterAsDRPCServer registers the API to the mux.
func (a *API) RegisterAsDRPCServer(mux drpc.Mux) {
	_ = DRPCRegisterHydraDaemonService(mux, a)
}

// _ is a type assertion
var _ DRPCHydraDaemonServiceServer = ((*API)(nil))

// Validate validates the operation code.
// Unknown is considered valid.
func (op BucketOp) Validate() error {
	switch op {
	case BucketOp_BucketOp_UNKNOWN:
	case BucketOp_BucketOp_BLOCK_GET:
	case BucketOp_BucketOp_BLOCK_PUT:
	case BucketOp_BucketOp_BLOCK_RM:
	default:
		return errors.Errorf("bucket op unknown: %v", op.String())
	}

	return nil
}

// Validate validates the request.
func (r *BucketOpRequest) Validate() error {
	if err := r.GetOp().Validate(); err != nil {
		return err
	}
	switch r.GetOp() {
	case BucketOp_BucketOp_BLOCK_RM:
		fallthrough
	case BucketOp_BucketOp_BLOCK_GET:
		if err := r.GetBlockRef().Validate(); err != nil {
			return errors.New("block ref must be specified")
		}
	case BucketOp_BucketOp_BLOCK_PUT:
		if len(r.GetData()) == 0 {
			return errors.New("empty block not allowed")
		}
		if err := r.GetPutOpts().Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate validates the operation code.
func (op ObjectStoreOp) Validate() error {
	switch op {
	case ObjectStoreOp_ObjectStoreOp_GET_KEY:
	case ObjectStoreOp_ObjectStoreOp_DELETE_KEY:
	case ObjectStoreOp_ObjectStoreOp_LIST_KEYS:
	case ObjectStoreOp_ObjectStoreOp_PUT_KEY:
	case ObjectStoreOp_ObjectStoreOp_UNKNOWN:
	default:
		return errors.Errorf("bucket op unknown: %v", op.String())
	}

	return nil
}

// Validate validates the request.
func (r *ObjectStoreOpRequest) Validate() error {
	if err := r.GetOp().Validate(); err != nil {
		return err
	}
	if r.GetStoreName() == "" {
		return errors.New("store name must be set")
	}
	if r.GetKey() == "" && r.GetOp() != ObjectStoreOp_ObjectStoreOp_LIST_KEYS {
		return errors.New("key must be set")
	}
	if r.GetVolumeId() == "" &&
		(r.GetOp() == ObjectStoreOp_ObjectStoreOp_DELETE_KEY ||
			r.GetOp() == ObjectStoreOp_ObjectStoreOp_PUT_KEY) {
		return errors.New("volume id must be set")
	}
	switch r.GetOp() {
	case ObjectStoreOp_ObjectStoreOp_PUT_KEY:
		if len(r.GetData()) == 0 {
			return errors.New("empty object not allowed")
		}
	}
	return nil
}
