package assembly_controller

import (
	"context"

	"github.com/aperturerobotics/bldr/assembly"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	// boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
)

// runningSubAssembly contains information about a running assembly.
// it is also the value for the directive ApplyAssembly
type runningSubAssembly struct {
	c      *Controller
	parent *runningAssembly

	// conf is the SubAssembly config
	conf assembly.SubAssembly
	// parent mtx guards the below fields.
	// state is the current state
	state runningSubAssemblyState
}

func newRunningSubAssembly(
	parent *runningAssembly,
	conf assembly.SubAssembly,
) *runningSubAssembly {
	return &runningSubAssembly{
		c:      parent.c,
		parent: parent,
		conf:   conf,
		state: runningSubAssemblyState{
			conf: conf,
		},
	}
}

// Execute actuates the running subassembly.
func (c *runningSubAssembly) Execute(ctx context.Context) error {
	// Return fatal errors only.
	le := c.c.le
	if id := c.conf.GetId(); id != "" {
		le = le.WithField("subassembly", id)
	}
	le.Info("subassembly routine starting")

	c.updateState(true, func(st *runningSubAssemblyState) bool {
		if st.asms == nil && st.err == nil {
			return false
		}
		st.asms = nil
		st.err = nil
		return true
	})

	// resolve directive bridges
	dirBridges, err := c.conf.ResolveDirectiveBridges(ctx, c.c.bus)
	if err != nil {
		return err
	}

	// resolve and apply assemblies
	asms, err := c.conf.ResolveAssemblies(ctx, c.c.bus)
	if err != nil {
		return err
	}

	// create sub-bus
	b, _, err := NewSubAssemblyBus(ctx, le)
	if err != nil {
		return err
	}

	// load configset controller
	csCtrl, err := configset_controller.NewController(le, b)
	if err != nil {
		return err
	}
	csRel, err := b.AddController(
		ctx,
		csCtrl,
		nil,
	)
	if err != nil {
		return err
	}
	defer csRel()

	// fatal errors
	errCh := make(chan error, 4)

	// run directive bridges
	if len(dirBridges) != 0 {
		go func() {
			errCh <- c.executeDirectiveBridges(ctx, b, dirBridges)
		}()
	}

	// start assembly controller
	asmCtrl, err := NewController(le, b, &Config{
		DisablePartialSuccess: c.c.c.GetDisablePartialSuccess(),
	})
	if err != nil {
		return err
	}
	go func() {
		errCh <- b.ExecuteController(ctx, asmCtrl)
	}()

	// apply assemblies
	asmRefs := make([]assembly.Reference, 0, len(asms))
	asmStates := make([]assembly.State, 0, len(asms))
	for _, asm := range asms {
		if asm == nil {
			continue
		}
		asmRef, err := asmCtrl.PushAssembly(ctx, asm)
		if err != nil {
			return err
		}
		asmRefs = append(asmRefs, asmRef)
		asmStates = append(asmStates, asmRef.GetState())
	}

	// check for immediate error
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		if err != nil {
			return err
		}
	default:
	}

	// set success
	c.updateState(true, func(st *runningSubAssemblyState) bool {
		st.err = nil
		st.asms = make([]assembly.State, len(asmStates))
		copy(st.asms, asmStates)
		return true
	})

	// add assembly state callbacks
	for i, asmRef := range asmRefs {
		asmRef.AddStateCb(func(st assembly.State) {
			c.parent.mtx.Lock()
			defer c.parent.mtx.Unlock()

			if asmStates[i].GetControllerStatus() == st.GetControllerStatus() &&
				asmStates[i].GetError() == st.GetError() {
				return
			}
			asmStates[i] = st
			c.updateState(false, func(rst *runningSubAssemblyState) bool {
				rst.asms = make([]assembly.State, len(asmStates))
				copy(rst.asms, asmStates)
				rst.err = nil
				return true
			})
		})
	}

	// wait for fatal error or exit
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
}

// updateState applies a change to the state.
// cb should return if dirty
func (c *runningSubAssembly) updateState(lock bool, cb func(st *runningSubAssemblyState) bool) {
	if lock {
		c.parent.mtx.Lock()
	}
	dirty := cb(&c.state)
	if dirty {
		c.parent.updateSubAssemblyStates()
		c.parent.emitState()
	}
	if lock {
		c.parent.mtx.Unlock()
	}
}

// executeDirectiveBridges manages executing the configured directive bridges.
// returns fatal error only or nil
func (c *runningSubAssembly) executeDirectiveBridges(
	ctx context.Context,
	target bus.Bus,
	bridges []assembly.DirectiveBridge,
) error {
	hostBus := c.c.bus
	errCh := make(chan error, 1)

	// construct and start all bridge controllers
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	for i, bridge := range bridges {
		ctrlConfCc := bridge.GetControllerConfig()
		if ctrlConfCc == nil || ctrlConfCc.GetConfig() == nil {
			// no-op
			continue
		}
		ctrlConf := ctrlConfCc.GetConfig()

		bridgeFrom, bridgeTo := hostBus, target
		if bridge.GetBridgeToParent() {
			bridgeFrom, bridgeTo = target, hostBus
		}

		// construct with the controller config
		cf, cfRef, err := resolver.ExLoadFactoryByConfig(subCtx, hostBus, ctrlConf)
		if err != nil {
			return errors.Wrapf(err, "directive_bridges[%d]: lookup factory for config %q", i, ctrlConf.GetConfigID())
		}
		if cf == nil {
			if cfRef != nil {
				cfRef.Release()
			}
			continue
		}

		ctrl, err := cf.Construct(ctrlConf, controller.ConstructOpts{
			Logger: c.c.le,
		})
		cfRef.Release()
		if err != nil {
			return errors.Wrapf(err, "directive_bridges[%d]: construct config %q", i, ctrlConf.GetConfigID())
		}

		dbc, dbcOk := ctrl.(assembly.DirectiveBridgeController)
		if !dbcOk {
			return errors.Wrapf(
				err,
				"directive_bridges[%d]: controller %q must implement DirectiveBridgeController",
				i, ctrlConf.GetConfigID(),
			)
		}
		dbc.SetDirectiveBridgeTarget(bridgeTo)

		// execute the bridge controller
		go func() {
			if err := bridgeFrom.ExecuteController(subCtx, dbc); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	}

	// wait for any fatal error or cancel
	for {
		select {
		case <-subCtx.Done():
			return subCtx.Err()
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
}
