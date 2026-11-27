// Package kernel runs Linux programs compiled to WebAssembly. Its host
// functions implement the Linux syscalls and WASI imports those programs
// call: processes, signals, pipes, sockets, the console and pseudo-terminals,
// and a filesystem over an optional root. No Linux kernel runs. The Go host
// is the kernel.
package kernel

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

type processContextKey struct{}

// Kernel is one running system: its WebAssembly runtime, filesystem,
// process table and console.
type Kernel struct {
	runtime engine.Runtime
	vfs     *VFS

	// mu is the kernel lock. Each host call holds it except while it
	// blocks, so processes change the kernel's state one at a time.
	mu sync.Mutex
	// caller is the process whose host call holds the kernel lock and
	// whose directory /proc/self selects.
	caller *Process
	// mounts is the mount table, in mount order. The kernel lock guards it.
	mounts []*mount
	// listens wakes the host dials that wait for a listener each time a
	// socket starts listening.
	listens broadcast.Broadcast

	// procs is the process table.
	procs *processTable
	// mapSeq numbers the file mappings in creation order. The kernel lock
	// guards it.
	mapSeq uint64
	// shutdown ends the run Boot started, with ErrReboot or errHalt as the
	// cause. It is nil outside Boot.
	shutdown context.CancelCauseFunc

	// console is the terminal of /dev/console, which each process starts
	// with as its standard input, output and error.
	console *tty
	// ptys holds the open pseudo-terminals by index, and pts is the
	// /dev/pts directory of their slaves. The kernel lock guards both.
	ptys map[int]*pty
	pts  *VNode

	// termFD is the host terminal's descriptor, or -1 for none.
	termFD int
	// termRaw reports whether the host terminal is in raw mode.
	termRaw atomic.Bool
	// termMakeRaw, termRestore and termGetSize operate the host terminal.
	// The platform's initTermOps sets them.
	termMakeRaw func(fd int) (any, error)
	termRestore func(fd int, state any) error
	termGetSize func(fd int) (int, int, error)
	// termOldState is the host terminal's state before raw mode, which Close
	// restores.
	termOldState any

	// debugSyscalls logs every syscall to stderr.
	debugSyscalls bool

	// bootTime is when the kernel started. The monotonic clocks count from
	// it, unaffected by setting the realtime clock.
	bootTime time.Time
	// timeMu guards realtimeOffset.
	timeMu sync.Mutex
	// realtimeOffset is added to the host time to give the kernel's realtime
	// clock.
	realtimeOffset time.Duration

	// initPath and initArgs are the program Boot runs.
	initPath string
	initArgs []string
	// env is the processes' environment.
	env map[string]string

	// hostname and domainname are the names uname reports.
	hostname   string
	domainname string
}

// New creates a kernel that runs its processes on rt. The kernel defines its
// host modules in rt, so rt serves this kernel alone.
func New(ctx context.Context, rt engine.Runtime, cfg *Config) (*Kernel, error) {
	// Choose the console's terminal. A virtual terminal replaces the host's.
	if cfg == nil {
		return nil, ErrNilConfig
	}
	termFD := cfg.TerminalFD
	if (termFD == 0 && cfg.Stdin == nil) || cfg.VirtualTerminal {
		termFD = -1
	}

	// Fill in the configuration defaults.
	env := cfg.Env
	if env == nil {
		env = DefaultEnv()
	}
	k := &Kernel{
		runtime:       rt,
		procs:         newProcessTable(),
		debugSyscalls: cfg.DebugSyscalls,
		termFD:        termFD,
		bootTime:      time.Now(),
		initPath:      cmp.Or(cfg.InitPath, "/init"),
		initArgs:      cfg.InitArgs,
		env:           env,
		hostname:      "spacewave",
		domainname:    "(none)",
	}

	// Create the console. Its output paths turn a newline into a carriage
	// return and newline while the terminal asks for it.
	initTermOps(k)
	k.console = newTTY(k, termFD >= 0 || cfg.VirtualTerminal, cfg.VirtualTerminal)
	k.console.stdout = &onlcrWriter{term: k.console, inner: cmp.Or[io.Writer](cfg.Stdout, os.Stdout)}
	k.console.stderr = &onlcrWriter{term: k.console, inner: cmp.Or[io.Writer](cfg.Stderr, os.Stderr)}

	// Set up the filesystem and the host modules.
	k.initVFS(cfg)
	if err := k.defineHostModules(ctx); err != nil {
		return nil, err
	}

	// Start reading the console input.
	k.console.in.start(ctx, cmp.Or[io.Reader](cfg.Stdin, os.Stdin))
	return k, nil
}

// Boot runs the program at InitPath as PID 1 and waits for it, or until the
// reboot call ends the run. It returns ErrReboot when the system restarted
// and nil when it halted.
func (k *Kernel) Boot(ctx context.Context) error {
	// Read the init program.
	node := k.vfs.lookup(k.initPath)
	if node == nil {
		return ErrNoInit
	}
	wasm, err := k.vfs.readFile(node)
	if err != nil || len(wasm) == 0 {
		return ErrNoInit
	}

	// Run it with its arguments until it exits or the reboot call ends the
	// run.
	ctx, shutdown := context.WithCancelCause(ctx)
	defer shutdown(nil)
	k.mu.Lock()
	k.shutdown = shutdown
	k.mu.Unlock()
	args := append([]string{k.initPath}, k.initArgs...)
	err = k.RunProcess(ctx, wasm, args...)

	// Report a restart. A halt ends the run as an exiting init does.
	switch cause := context.Cause(ctx); {
	case errors.Is(cause, ErrReboot):
		return ErrReboot
	case errors.Is(cause, errHalt):
		return nil
	}
	return err
}

// RunProcess runs a guest program as a new process and waits for it and every
// child it started. A nonzero exit returns a ProcessExitError.
func (k *Kernel) RunProcess(ctx context.Context, wasm []byte, args ...string) error {
	// Register the process under a fresh PID.
	pid := k.procs.allocPID()
	p := newProcess(k, pid, args)
	if len(args) != 0 {
		// Name the program by its absolute path, as /proc/self/exe does.
		exe := cmp.Or(p.lookPath(args[0]), args[0])
		p.exe = resolvePath(p.cwd, p.fds, atFDCWD, exe)
	}
	k.procs.set(pid, p)

	// Run it, then wait for the children it left running.
	err := k.runProcess(ctx, p, func(ctx context.Context) error {
		return k.runProgram(ctx, p, wasm)
	})
	if err != nil {
		return err
	}
	if err := k.procs.waitIdle(ctx); err != nil {
		return err
	}
	if p.exitCode != 0 {
		return &ProcessExitError{Code: p.exitCode}
	}
	return nil
}

// startChildProcess runs a registered child process in the background. run
// runs its guest, as runProcess describes.
func (k *Kernel) startChildProcess(ctx context.Context, p *Process, run func(ctx context.Context) error) {
	k.procs.start(func() {
		_ = k.runProcess(ctx, p, run)
	})
}

// runProcess runs a registered process to completion with run, records its
// exit and releases it. run runs the guest until it returns or exits. A
// program that cannot run exits with 126. A child reports that error on the
// kernel's stderr, since its parent sees only the exit code.
func (k *Kernel) runProcess(ctx context.Context, p *Process, run func(ctx context.Context) error) error {
	// Give the process a context its kill and exit cancel. A terminating
	// signal that arrived before the start cancels it at once.
	procCtx, cancel := context.WithCancel(context.WithValue(ctx, processContextKey{}, p))
	p.sigMu.Lock()
	p.cancel = cancel
	killed := p.sigTerm != 0
	p.sigMu.Unlock()
	if killed {
		cancel()
	}
	defer k.exitProcess(p)

	// Run the program. A terminating signal ends it with status 128 plus the
	// signal number. Report any other failure before the parent can see it.
	err := k.runGuest(procCtx, p, run)
	p.sigMu.Lock()
	sig := p.sigTerm
	p.sigMu.Unlock()
	switch {
	case sig != 0:
		p.exitCode = 128 + int(sig)
		p.exitSignal = sig
		return nil
	case err != nil:
		p.exitCode = exitCannotRun
		if p.ppid != 0 {
			_, _ = fmt.Fprintf(k.console.stderr, "pid %d: %v\n", p.pid, err)
		}
	}
	return err
}

// runGuest runs the process's guest with run and records its exit code. When
// the program replaced itself with exec, the exit code is the replacement's.
// It returns an error when the program could not run or trapped.
func (k *Kernel) runGuest(ctx context.Context, p *Process, run func(ctx context.Context) error) error {
	// Run the guest, which ends by panicking a ProcessExitError from exit.
	err := run(ctx)
	if p.inst != nil {
		k.mu.Lock()
		p.unmapAll()
		k.mu.Unlock()
		_ = p.inst.Close(ctx)
	}
	var exitErr *ProcessExitError
	if !errors.As(err, &exitErr) {
		return err
	}
	p.exitCode = exitErr.Code

	// After exec, the replacement process carries on under this PID.
	if child := p.selfExecProc; child != nil {
		<-child.exitCh
		p.exitCode = child.exitCode
		p.exitSignal = child.exitSignal
	}
	return nil
}

// runProgram compiles wasm as the process's program and runs it from its
// entry point.
func (k *Kernel) runProgram(ctx context.Context, p *Process, wasm []byte) error {
	// Compile and instantiate the program.
	wasm, globals, err := prepareGuest(wasm)
	if err != nil {
		return err
	}
	compiled, err := k.runtime.Compile(ctx, wasm)
	if err != nil {
		return errors.Wrap(err, "compile")
	}
	inst, err := compiled.Instantiate(ctx, "proc-"+strconv.Itoa(p.pid))
	if err != nil {
		return errors.Wrap(err, "instantiate")
	}
	p.inst, p.module, p.globals = inst, compiled, globals
	return p.start(ctx)
}

// wait waits for ch to close and reports true, or for ctx to end and reports
// false, without the kernel lock. A ctx that has already ended returns at
// once, keeping the lock.
func wait(ctx context.Context, ch <-chan struct{}) bool {
	if ctx.Err() != nil {
		return false
	}
	done := false
	blocking(ctx, func() {
		select {
		case <-ctx.Done():
		case <-ch:
			done = true
		}
	})
	return done
}

// waitAny waits without the kernel lock for ctx to end, timer to fire or any
// of changes to close, and returns waitDone, waitTimer or waitChange. A nil
// timer never fires.
func waitAny(ctx context.Context, timer <-chan time.Time, changes []<-chan struct{}) int {
	// List the cases in the order of the results.
	cases := make([]reflect.SelectCase, 0, len(changes)+2)
	cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())})
	cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(timer)})
	for _, change := range changes {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(change)})
	}

	// Wait for one.
	var chosen int
	blocking(ctx, func() { chosen, _, _ = reflect.Select(cases) })
	return min(chosen, waitChange)
}

// The results of waitAny.
const (
	waitDone = iota
	waitTimer
	waitChange
)

// blocking runs wait, which blocks, without the kernel lock the host call
// running under ctx holds, so other processes run in the meantime. Outside a
// process's host call it runs wait as is.
func blocking(ctx context.Context, wait func()) {
	// Find the process of the host call.
	p, ok := ctx.Value(processContextKey{}).(*Process)
	if !ok {
		wait()
		return
	}

	// Release the lock while waiting.
	k := p.kernel
	k.mu.Unlock()
	defer func() {
		k.mu.Lock()
		k.caller = p
	}()
	wait()
}

// exitCannotRun is the exit code of a program that cannot run, as a shell
// reports a command it cannot execute.
const exitCannotRun = 126

// exitProcess releases a process after its program returns: it closes the
// process's descriptors, wakes its waiters, gives its children to init and
// sends its parent SIGCHLD. When no parent will reap it, it removes it from
// the process table.
func (k *Kernel) exitProcess(p *Process) {
	// Close the descriptors under the kernel lock, which guards their open
	// files, and record the exit.
	k.mu.Lock()
	defer k.mu.Unlock()
	defer k.procs.exited()
	for fd, of := range p.fds {
		of.unref()
		delete(p.fds, fd)
	}
	clear(p.cloexec)
	if p.realTimer != nil {
		p.realTimer.stop()
	}
	p.cancel()
	close(p.exitCh)

	// An exec's replacement keeps the children and tells the parent later.
	if k.procs.get(p.pid) != p {
		return
	}

	// Give the children to init, which reaps the exited ones.
	if init := k.procs.get(initPID); init != nil && init != p {
		var reap bool
		for _, c := range k.procs.filter(func(c *Process) bool { return c.ppid == p.pid }) {
			c.ppid = initPID
			reap = reap || c.exited()
		}
		if reap {
			init.deliverSignal(SIGCHLD)
		}
	}

	// Tell the parent, or forget a process no parent reaps.
	if p.ppid == 0 {
		k.procs.removeProcess(p)
	} else if parent := k.procs.get(p.ppid); parent != nil {
		parent.deliverSignal(SIGCHLD)
	}
}

// initPID is the PID of the kernel's first process, which adopts orphans.
const initPID = 1

// Close stops reading the console, restores the host terminal and closes the
// runtime.
func (k *Kernel) Close(ctx context.Context) error {
	// Stop the input and restore the terminal.
	k.console.in.close()
	if k.termOldState != nil && k.termRestore != nil {
		_ = k.termRestore(k.termFD, k.termOldState)
		k.termOldState = nil
		k.termRaw.Store(false)
	}

	// Release the runtime.
	return k.runtime.Close(ctx)
}

// Resize sets the console's window size and sends SIGWINCH to its
// foreground processes when the size changed.
func (k *Kernel) Resize(cols, rows uint32) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.console.resize(cols, rows)
}

// Signal sends sig to the console's foreground processes, as the host
// forwards a signal it received, such as SIGINT from a terminal in cooked
// mode.
func (k *Kernel) Signal(sig uint32) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.console.signalForeground(sig)
}

// initVFS creates the filesystem: the devices, the in-memory directories, the
// root filesystem and the configured files. The console's node joins the
// console k already has.
func (k *Kernel) initVFS(cfg *Config) {
	// Create a VFS over the root filesystem.
	k.vfs = newVFS()
	v := k.vfs
	v.host = cfg.RootFS

	// Add the devices.
	dev := v.mkDir(v.root, "dev")
	v.mkCharDev(dev, "null", devNull)
	v.mkCharDev(dev, "zero", devZero)
	v.mkCharDev(dev, "urandom", devURandom)

	// Add the terminals: the console, the controlling terminal and the
	// pseudo-terminal multiplexer with its empty table of slaves.
	k.console.node = v.mkCharDev(dev, "console", devConsole)
	k.console.node.tty = k.console
	v.mkCharDev(dev, "tty", devTTY)
	v.mkCharDev(dev, "ptmx", devPtmx)
	k.pts, k.ptys = v.mkDir(dev, "pts"), make(map[int]*pty)

	// Add the shared memory directory, where shm_open opens its files.
	v.mkDir(dev, "shm")

	// Add the in-memory directories that overlay the root filesystem, and
	// mount the root and the proc and sys filesystems as init would.
	v.mkDir(v.root, "tmp")
	k.mounts = []*mount{{source: "rootfs", target: "/", fstype: "rootfs", root: v.root}}
	k.mountFS(v.mkDir(v.root, "proc"), &mount{source: "proc", target: "/proc", fstype: "proc", root: k.newProcFS()})
	k.mountFS(v.mkDir(v.root, "sys"), &mount{source: "sysfs", target: "/sys", fstype: "sysfs", root: k.newSysFS()})

	// List the root filesystem's top level beside the overlays, or create
	// the standard directories of an in-memory root.
	if v.host != nil {
		v.loadHostChildren(v.root, "/")
	} else {
		v.mkDir(v.root, "etc")
		v.mkDir(v.root, "bin")
		usr := v.mkDir(v.root, "usr")
		v.mkDir(usr, "bin")
		v.mkDir(v.root, "root")
	}

	// Add the configured files. A WebAssembly program is executable.
	for path, content := range cfg.Files {
		perm := uint32(0o644)
		if bytes.HasPrefix(content, wasmMagic) {
			perm = 0o755
		}
		v.addFile(path, content, perm)
	}
}
