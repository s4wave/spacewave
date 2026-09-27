//go:build !js

package daemon

import (
	stderrors "errors"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

// process retains only the child created by one startup attempt. Its caller
// must call stop or release exactly once and must not otherwise wait on cmd.
type process struct {
	// cmd owns the child process until stop or release.
	cmd *exec.Cmd
	// kill terminates the unready child tree before cmd can reap its leader.
	kill func() error
	// killChild terminates the retained child after tree termination fails.
	// os.ErrProcessDone means exit is established and cmd.Wait can join it.
	killChild func() error
	// detach releases platform tracking without stopping the child.
	detach func() error
}

// stop terminates and joins this unready child, then releases platform tracking.
// If both termination attempts fail without a verified exit, it releases handles
// without waiting and reports that the unacknowledged child may remain alive.
// No shared socket participates, since it may belong to another starter.
func (p *process) stop() error {
	// Retain the unreaped leader's identity through tree termination. Even an
	// already exited Unix child reserves its PID until Wait reaps it.
	treeErr := p.kill()
	if treeErr != nil {
		// Make one direct-child attempt before relinquishing its identity.
		// A refused signal cannot justify waiting indefinitely for a live child.
		if err := p.killChild(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return errors.Wrap(stderrors.Join(treeErr, err, p.release()),
				"unacknowledged daemon process may remain until the OS or child exits")
		}
	}

	// A failed exit status is expected during failed-start cleanup; retain
	// actual wait errors and release platform tracking only after joining.
	err := p.cmd.Wait()
	if _, ok := stderrors.AsType[*exec.ExitError](err); ok {
		err = nil
	}
	return stderrors.Join(treeErr, err, p.detach())
}

// release relinquishes custody without terminating or waiting for the child.
// It is used after readiness or when the OS refuses failed-start termination.
func (p *process) release() error {
	return stderrors.Join(p.detach(), p.cmd.Process.Release())
}
