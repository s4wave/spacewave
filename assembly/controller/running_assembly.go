package assembly_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bldr/assembly"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
)

// runningAssembly contains information about a running assembly.
// it is also the value for the directive ApplyAssembly
type runningAssembly struct {
	c *Controller
	// ctxCancel is the context cancel, nil until just before Execute is called
	// set by the controller, guarded by controller mtx
	ctxCancel context.CancelFunc
	// conf is the Assembly config
	conf assembly.Assembly

	// mtx guards the state
	mtx sync.Mutex
	// state is the current state
	state runningAssemblyState
	// ref is the reference
	ref *runningAssemblyRef
	// runningSubAsms is the list of running sub assemblies
	runningSubAsms []*runningSubAssembly
}

func newRunningAssembly(
	c *Controller,
	conf assembly.Assembly,
) (*runningAssembly, *runningAssemblyRef) {
	rc := &runningAssembly{
		c:    c,
		conf: conf,
	}
	rc.ref = newRunningAssemblyRef(rc)
	return rc, rc.ref
}

// GetControllerConfig returns the controller config in use.
// The value will be revoked and re-emitted if this changes.
func (c *runningAssembly) GetAssembly() assembly.Assembly {
	return c.conf
}

// GetState returns the current state object.
func (c *runningAssembly) GetState() assembly.State {
	c.mtx.Lock()
	st := c.state
	c.mtx.Unlock()
	return &st
}

// Execute actuates the running controller.
// must be called after ctxCancel field is set.
func (c *runningAssembly) Execute(ctx context.Context) (rerr error) {
	le := c.c.le
	le.Debug("starting assembly")

	defer func() {
		if rerr != nil && rerr != context.Canceled {
			le.WithError(rerr).Warn("assembly failed")
		}
		c.ctxCancel()
		c.c.releaseAssembly(c)
	}()

	// execute controller exec
	conf := c.conf
	errCh := make(chan error, 2)
	go func() {
		errCh <- c.executeControllerExec(ctx, conf)
	}()

	// execute sub-assembles
	go func() {
		errCh <- c.executeSubAssemblies(ctx, conf)
	}()

	nrunning := 2
	var lastErr error
	for nrunning > 0 {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			if err != nil {
				lastErr = err
				if c.c.c.GetDisablePartialSuccess() {
					return err
				}
			}
			nrunning--
		}
	}
	return lastErr
}

// executeControllerExec executes the ControllerExec field.
// return an error to indicate failure of the assembly.
func (c *runningAssembly) executeControllerExec(ctx context.Context, asm assembly.Assembly) error {
	execReq, err := asm.ResolveControllerExec(ctx, c.c.bus)
	if err != nil {
		return err
	}
	if execReq == nil {
		return nil
	}
	return execReq.Execute(
		ctx,
		c.c.bus,
		c.c.c.GetDisablePartialSuccess(),
		func(resp *controller_exec.ExecControllerResponse) error {
			c.mtx.Lock()
			var dirty bool
			nerr := resp.GetError()
			if ((nerr == nil) != (c.state.err == nil)) ||
				(nerr != nil && c.state.err != nil && nerr.Error() != c.state.err.Error()) {
				dirty = true
				c.state.err = nerr
			}
			nstat := resp.GetStatus()
			if nstat != c.state.cStat {
				dirty = true
				c.state.cStat = nstat
			}
			if dirty {
				c.emitState()
			}
			c.mtx.Unlock()
			return nil
		},
	)
}

// executeSubAssemblies executes the SubAssemblies field.
// return an error to indicate failure of the assembly.
func (c *runningAssembly) executeSubAssemblies(ctx context.Context, asm assembly.Assembly) error {
	subAsms, err := asm.ResolveSubAssemblies(ctx, c.c.bus)
	if err != nil || len(subAsms) == 0 {
		return err
	}
	errCh := make(chan error, len(subAsms))
	c.mtx.Lock()
	var dirty bool

	for _, subAsm := range subAsms {
		if subAsm == nil {
			continue
		}
		asm := newRunningSubAssembly(c, subAsm)
		c.runningSubAsms = append(c.runningSubAsms, asm)
		dirty = true
		go func() {
			err := asm.Execute(ctx)
			if err != nil && err != context.Canceled {
				asm.updateState(true, func(st *runningSubAssemblyState) bool {
					if st.err == err {
						return false
					}
					st.err = err
					st.asms = nil
					return true
				})
			}
			errCh <- err
		}()
	}
	if dirty {
		c.updateSubAssemblyStates()
		c.emitState()
	}
	c.mtx.Unlock()

	nrunning := len(subAsms)
	var lastErr error
	for nrunning > 0 {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			if err != nil {
				if err != context.Canceled {
					c.c.le.WithError(err).Warn("sub-assembly errored")
				}
				lastErr = err
				if c.c.c.GetDisablePartialSuccess() {
					return err
				}
			}
			nrunning--
		}
	}
	return lastErr
}

// updateSubAssemblyStates rebuilds the list of subassembly states.
// expects mtx to be locked
func (c *runningAssembly) updateSubAssemblyStates() {
	states := make([]assembly.SubAssemblyState, len(c.runningSubAsms))
	for i, subAsm := range c.runningSubAsms {
		st := subAsm.state // creates a copy
		states[i] = &st
	}
	c.state.subAsm = states
}

// emitState emits the updated state.
// expects mtx to be locked
func (c *runningAssembly) emitState() {
	st := c.state
	c.ref.pushState(&st)
}

// Release stops and removes the assembly.
func (c *runningAssembly) Release() {
	c.c.releaseAssembly(c)
}
