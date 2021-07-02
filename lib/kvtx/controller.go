package forge_lib_kvtx

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	kvtx_block "github.com/aperturerobotics/hydra/kvtx/block"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/kvtx/1"

// inputNameStore is the name of the Input for the store.
const inputNameStore = "store"

// outputNameStore is the name of the Output for the store.
const outputNameStore = inputNameStore // "store"

// Controller implements the kvtx operations controller.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the configuration
	conf *Config
	// inputVals is the input values map
	inputVals forge_value.ValueMap
	// handle contains the controller handle
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new kvtx ops execution controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() controller.Info {
	return controller.Info{
		Id:      ControllerID,
		Version: Version.String(),
	}
}

// InitForgeExecController initializes the Forge execution controller.
// This is called before Execute().
// Any error returned cancels execution of the controller.
func (c *Controller) InitForgeExecController(
	ctx context.Context,
	inputVals forge_value.ValueMap,
	handle forge_target.ExecControllerHandle,
) error {
	c.inputVals, c.handle = inputVals, handle
	return c.conf.Validate()
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	var err error
	conf := c.conf

	// base operations list
	ops := append([]*Op{}, conf.GetOps()...)

	// fetch additional config input
	confInput, err := c.fetchConfigInput(ctx)
	if err != nil {
		return err
	}

	ops = append(ops, confInput.GetOps()...)
	if len(ops) == 0 {
		return nil
	}

	var rootRef *bucket.ObjectRef
	inStore := c.inputVals[inputNameStore]
	if !inStore.IsEmpty() {
		rootRef, err = inStore.ToBucketRef()
		if err != nil {
			return errors.Wrap(err, "load store input value")
		}
	}

	// resolve the op keys and/or values
	opQueue := NewOpQueue(ctx, c.inputVals, c.handle)

	// add configured operation list
	err = opQueue.AddOps(ops)
	if err != nil {
		return err
	}

	// apply operations
	var nextRootRef *block.BlockRef
	var sizeBefore, sizeAfter uint64
	// access the kvtx tree. note this might occur cross-bucket.
	opCount := len(opQueue.GetPendingOps())
	err = c.handle.AccessStorage(
		ctx,
		rootRef,
		func(cs *bucket_lookup.Cursor) error {
			if rootRef.GetEmpty() {
				c.le.
					WithField("store-type", kvtx_block.DefaultKeyValueStoreImpl.String()).
					Info("store input was empty, initializing empty store")
			}
			btx, bcs := cs.BuildTransactionAtRef(nil, rootRef.GetRootRef())
			kvtx, berr := kvtx_block.BuildKvTransaction(ctx, bcs, true)
			if berr != nil {
				return berr
			}
			defer kvtx.Discard()
			sizeBefore, berr = kvtx.Size()
			if berr != nil {
				return err
			}
			berr = opQueue.ApplyOps(kvtx, true, c.conf.GetIgnoreErrors())
			if berr == nil {
				sizeAfter, berr = kvtx.Size()
			}
			if berr == nil {
				berr = kvtx.Commit(ctx)
			}
			if berr != nil {
				return berr
			}
			nextRootRef, _, berr = btx.Write(true)
			return berr
		},
	)
	if err != nil {
		return err
	}

	// set updated store as output
	le := c.le
	var outpRef *bucket.ObjectRef
	if rootRef.GetEmpty() {
		outpRef = &bucket.ObjectRef{}
	} else {
		outpRef = rootRef.Clone()
		le = le.
			WithField("root-ref-before", outpRef.GetRootRef().MarshalString()).
			WithField("size-before", sizeBefore)
	}
	outpRef.RootRef = nextRootRef
	le = le.
		WithField("root-ref-after", nextRootRef.MarshalString()).
		WithField("ref-after", outpRef.MarshalString()).
		WithField("size-after", sizeAfter)
	le.Infof("applied %d ops to store", opCount)

	outpSlice := []*forge_value.Value{{
		Name:      outputNameStore,
		ValueType: forge_value.ValueType_ValueType_BUCKET_REF,
		BucketRef: outpRef,
	}}
	err = c.handle.SetOutputs(ctx, outpSlice, false)
	if err != nil {
		return err
	}

	// done
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) (directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// fetchConfigInput fetches the ConfigInput from a value.
func (c *Controller) fetchConfigInput(ctx context.Context) (*ConfigInput, error) {
	configInputName := c.conf.GetConfigInput()
	if len(configInputName) == 0 {
		return nil, nil
	}
	val, ok := c.inputVals[configInputName]
	if !ok {
		return nil, errors.Wrap(forge_value.ErrUnsetValue, configInputName)
	}
	return FetchConfigInput(ctx, c.handle, val)
}

// _ is a type assertion
var _ forge_target.ExecController = ((*Controller)(nil))
