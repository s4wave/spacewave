//go:build !js && !windows

package nativehost

import (
	"context"
	"encoding/binary"
	stderrors "errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pkg/errors"
	native "github.com/s4wave/spacewave/sdk/viewer/native"
	"golang.org/x/term"
)

const stopDeadline = 2 * time.Second
const terminalRestore = "\x1b[0m\x1b[?25h\x1b[?1049l"

// Host owns native child attempts and endpoint lifetimes.
type Host struct {
	config Config
	launch []byte
}

func NewHost(c Config) (*Host, error) {
	if !filepath.IsAbs(c.Executable) {
		return nil, errors.Errorf("native viewer executable must be absolute")
	}
	st, err := os.Stat(c.Executable)
	if err != nil {
		return nil, errors.Wrap(err, "native viewer executable")
	}
	if !st.Mode().IsRegular() {
		return nil, errors.Errorf("native viewer executable is not regular")
	}
	if st.Mode()&0111 == 0 {
		return nil, errors.Errorf("native viewer executable is not executable")
	}
	if c.LaunchRecord == nil {
		return nil, errors.Errorf("launch record is required")
	}
	if err := native.ValidateLaunchRecord(c.LaunchRecord); err != nil {
		return nil, err
	}
	if c.Stdin == nil || c.Stdout == nil || c.Stderr == nil {
		return nil, errors.Errorf("stdin, stdout, and stderr are required")
	}
	if err := validateTerminalFiles(c.Stdin, c.Stdout); err != nil {
		return nil, err
	}
	if c.EndpointFactory == nil {
		return nil, errors.Errorf("endpoint factory is required")
	}
	b, err := c.LaunchRecord.MarshalVT()
	if err != nil {
		return nil, err
	}
	return &Host{config: c, launch: b}, nil
}

func validateTerminalFiles(input, output *os.File) error {
	if !term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return errors.New("native viewer input and output must be terminals")
	}
	var inputStat, outputStat syscall.Stat_t
	if err := syscall.Fstat(int(input.Fd()), &inputStat); err != nil {
		return err
	}
	if err := syscall.Fstat(int(output.Fd()), &outputStat); err != nil {
		return err
	}
	if inputStat.Rdev != outputStat.Rdev {
		return errors.New("native viewer input and output must be the same terminal")
	}
	return nil
}

func (h *Host) Run(ctx context.Context, onReady func()) (runErr error) {
	called := false
	callback := func() {
		if !called {
			called = true
			if onReady != nil {
				onReady()
			}
		}
	}
	state, _ := term.GetState(int(h.config.Stdin.Fd()))
	defer func() {
		runErr = stderrors.Join(runErr, restoreTerminal(h.config.Stdout))
		if state != nil {
			runErr = stderrors.Join(runErr, term.Restore(int(h.config.Stdin.Fd()), state))
		}
		if x := recover(); x != nil {
			runErr = stderrors.Join(runErr, errors.Errorf("native viewer panic: %v", x))
		}
	}()
	var lastErr error
	for attempt := uint(0); attempt <= h.config.RestartLimit; attempt++ {
		err := h.attempt(ctx, callback)
		// Restore after every attempt, retaining errors across successful retries.
		runErr = stderrors.Join(runErr, restoreTerminal(h.config.Stdout))
		if state != nil {
			runErr = stderrors.Join(runErr, term.Restore(int(h.config.Stdin.Fd()), state))
		}
		if err == nil {
			return runErr
		}
		lastErr = err
		if ctx.Err() != nil {
			return stderrors.Join(runErr, err, ctx.Err())
		}
	}
	return stderrors.Join(runErr, lastErr)
}

func validateEndpoints(e *EndpointSet) error {
	if e == nil {
		return errors.Errorf("endpoint factory returned nil")
	}
	if e.Resource == nil || e.State == nil || e.Control == nil {
		return errors.Errorf("endpoint set contains nil endpoint")
	}
	resourceFD, stateFD, controlFD := e.Resource.Fd(), e.State.Fd(), e.Control.Fd()
	if resourceFD == stateFD || resourceFD == controlFD || stateFD == controlFD {
		return errors.Errorf("endpoint set contains aliased endpoints")
	}
	return nil
}

func (h *Host) attempt(ctx context.Context, onReady func()) (ret error) {
	recordR, recordW, err := os.Pipe()
	if err != nil {
		return err
	}
	readyR, readyW, err := os.Pipe()
	if err != nil {
		recordR.Close()
		recordW.Close()
		return err
	}
	defer recordR.Close()
	defer recordW.Close()
	defer readyR.Close()
	defer readyW.Close()
	eps, err := h.config.EndpointFactory(ctx)
	if err != nil {
		return err
	}
	if err := validateEndpoints(eps); err != nil {
		return stderrors.Join(err, eps.closeAndWait())
	}
	defer func() { ret = stderrors.Join(ret, eps.closeAndWait()) }()
	// #nosec G204: NewHost validates the configured absolute executable.
	cmd := exec.Command(h.config.Executable)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = h.config.Stdin, h.config.Stdout, h.config.Stderr
	cmd.ExtraFiles = []*os.File{recordR, readyW, eps.Resource, eps.State, eps.Control}
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "start native viewer")
	}
	recordR.Close()
	readyW.Close()
	eps.Resource.Close()
	eps.State.Close()
	eps.Control.Close()
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	reaped := false
	reap := func(cause error) error {
		if reaped {
			return cause
		}
		err := h.stop(cmd, wait, cause)
		reaped = true
		return err
	}
	defer func() {
		if !reaped {
			ret = stderrors.Join(ret, reap(nil))
		}
	}()
	var prefix [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(prefix[:], uint64(len(h.launch)))
	if _, err := recordW.Write(prefix[:n]); err != nil {
		return reap(err)
	}
	if _, err := recordW.Write(h.launch); err != nil {
		return reap(err)
	}
	recordW.Close()
	rr := make(chan struct {
		r   *native.NativeViewerReadinessRecord
		err error
	}, 1)
	go func() {
		r, e := native.ReadReadinessRecordLive(readyR, h.config.LaunchRecord)
		rr <- struct {
			r   *native.NativeViewerReadinessRecord
			err error
		}{r, e}
	}()
	ready := false
	var waitErr error
	classify := func(x struct {
		r   *native.NativeViewerReadinessRecord
		err error
	}) error {
		if x.err != nil {
			return errors.Wrap(x.err, "readiness")
		}
		if x.r.GetStatus() != native.NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY {
			return errors.Errorf("viewer readiness %s: %s", x.r.GetStatus().String(), x.r.GetDetail())
		}
		if !ready {
			ready = true
			if onReady != nil {
				onReady()
			}
		}
		return nil
	}
	for {
		select {
		case x := <-rr:
			readyR.Close()
			if err := classify(x); err != nil {
				return reap(err)
			}
		case waitErr = <-wait:
			reaped = true
			// A child can exit immediately after writing readiness. Drain the
			// buffered result before deciding that it exited too soon.
			if !ready {
				x := <-rr
				if err := classify(x); err != nil {
					return stderrors.Join(err, waitErr)
				}
				readyR.Close()
			}
			return waitErr
		case <-ctx.Done():
			return reap(ctx.Err())
		}
	}
}

func (h *Host) stop(cmd *exec.Cmd, wait <-chan error, cause error) error {
	if cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
	}
	timer := time.NewTimer(stopDeadline)
	defer timer.Stop()
	select {
	case err := <-wait:
		return stderrors.Join(cause, err)
	case <-timer.C:
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return stderrors.Join(cause, <-wait)
	}
}

func restoreTerminal(f *os.File) error {
	if f == nil {
		return nil
	}
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	_, err = io.WriteString(f, terminalRestore)
	return err
}
