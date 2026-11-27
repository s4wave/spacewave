package kernel

import (
	"bytes"
	"cmp"
	"context"
	"math/bits"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// posixFork starts a child process that runs beside the caller from a copy
// of its memory and globals. The caller's fork returns the child's PID, and
// the child's returns 0. A signal handler cannot fork: the engine resumes a
// fork only in the outermost guest call.
func (p *Process) posixFork(ctx context.Context) uint64 {
	// Capture the call stack.
	snapshotter := p.kernel.runtime.Snapshotter(ctx)
	if snapshotter == nil || p.handlerDepth != 0 {
		return negErrno(ENOSYS)
	}
	snapshot := snapshotter.Snapshot()

	// Copy the instance.
	pid := p.kernel.procs.allocPID()
	inst, err := p.module.Instantiate(ctx, "proc-"+strconv.Itoa(pid))
	if err != nil {
		return negErrno(ENOMEM)
	}
	if !copyInstance(inst, p.inst, p.globals) {
		_ = inst.Close(ctx)
		return negErrno(ENOMEM)
	}

	// Start the child at the fork's return.
	child := p.forkChild(pid, inst)
	p.kernel.procs.set(pid, child)
	p.kernel.startChildProcess(context.WithoutCancel(ctx), child, func(ctx context.Context) error {
		_, err := snapshot.Resume(ctx, inst, []uint64{0})
		return err
	})
	return uint64(pid)
}

// copyInstance copies the memory and the named globals of src into dst, a
// new instance of the same module. It reports false when dst cannot grow to
// the size of src.
func copyInstance(dst, src engine.Instance, globals []string) bool {
	// Grow the memory and copy it.
	srcMem, dstMem := src.Memory(), dst.Memory()
	if size := dstMem.Size(); size < srcMem.Size() {
		if _, ok := dstMem.Grow((srcMem.Size() - size) / wasmPageSize); !ok {
			return false
		}
	}
	view, ok := srcMem.Read(0, srcMem.Size())
	if !ok || !dstMem.Write(0, view) {
		return false
	}

	// Copy the globals.
	for _, name := range globals {
		if g := dst.ExportedGlobal(name); g != nil {
			g.Set(src.ExportedGlobal(name).Get())
		}
	}
	return true
}

// forkChild returns the process of a fork child running in inst. It copies
// p's process state, referencing each open file again, and p's signal
// dispositions and mask. The child has no pending signals, as on Linux.
func (p *Process) forkChild(pid int, inst engine.Instance) *Process {
	// Copy the process state.
	child := &Process{
		procState: p.clone(),
		kernel:    p.kernel,
		inst:      inst,
		module:    p.module,
		globals:   p.globals,
		jmpBufs:   p.jmpBufs.clone(),
		exitCh:    make(chan struct{}),
	}
	child.pid = pid
	child.ppid = p.pid
	for _, of := range child.fds {
		of.ref()
	}
	p.forkMappings(child)

	// Copy the signal state without the signals in flight.
	p.sigMu.Lock()
	child.sigState = p.sigState
	p.sigMu.Unlock()
	child.sigCaught = 0
	child.sigPending = 0
	return child
}

// sysExecve runs the program at pathPtr in place of the calling program.
func (p *Process) sysExecve(ctx context.Context, pathPtr, argvPtr, envpPtr uint32) uint64 {
	// Read the path, arguments and environment, then run the program.
	mem := p.inst.Memory()
	path := readCString(mem, pathPtr)
	args := readArgvFromMemory(mem, argvPtr)
	env := readEnvpFromMemory(mem, envpPtr)
	return p.exec(ctx, path, args, env)
}

// exec runs the program at path in place of the calling program, with env
// as its environment when env is set. It returns only an errno: on success
// the process exits for its replacement, which the caller waits on.
func (p *Process) exec(ctx context.Context, path string, args []string, env map[string]string) uint64 {
	// Read the program.
	resolved, wasm, args, errno := p.loadProgram(path, args)
	if errno != 0 {
		return errno
	}

	// Replace the process.
	p.execChild(args, env, resolved)
	p.unmapAll()
	_ = p.inst.Close(ctx)
	p.kernel.startChildProcess(context.WithoutCancel(ctx), p.selfExecProc, func(ctx context.Context) error {
		return p.kernel.runProgram(ctx, p.selfExecProc, wasm)
	})
	panic(&ProcessExitError{})
}

// maxInterpDepth is how many script interpreters an exec follows, as Linux
// limits binfmt_script recursion.
const maxInterpDepth = 4

// loadProgram reads the WebAssembly program that exec runs for path and args,
// returning its resolved path and arguments. A script that starts with "#!"
// runs its interpreter with the script's path in place of args[0], as Linux
// binfmt_script does.
func (p *Process) loadProgram(path string, args []string) (string, []byte, []string, uint64) {
	for range maxInterpDepth + 1 {
		// Read the file, which must be an executable regular file.
		resolved := resolvePath(p.cwd, p.fds, atFDCWD, path)
		node := p.kernel.vfs.lookup(resolved)
		if node == nil {
			return "", nil, nil, negErrno(ENOENT)
		}
		if node.mode&sIFMT != sIFREG || node.mode&0o111 == 0 {
			return "", nil, nil, negErrno(EACCES)
		}
		data, err := p.kernel.vfs.readFile(node)
		if err != nil {
			return "", nil, nil, negErrno(EIO)
		}
		if bytes.HasPrefix(data, wasmMagic) {
			return resolved, data, args, 0
		}

		// Run a script's interpreter on it.
		interp, arg, ok := parseInterpLine(data)
		if !ok {
			return "", nil, nil, negErrno(ENOEXEC)
		}
		next := []string{interp}
		if arg != "" {
			next = append(next, arg)
		}
		next = append(next, path)
		if len(args) > 1 {
			next = append(next, args[1:]...)
		}
		path, args = interp, next
	}
	return "", nil, nil, negErrno(ELOOP)
}

// interpLineMax is the most bytes of a script's "#!" line Linux reads.
const interpLineMax = 256

// parseInterpLine reads the interpreter and its optional single argument from
// a script's "#!" line.
func parseInterpLine(data []byte) (string, string, bool) {
	// Take the line after "#!".
	line, ok := bytes.CutPrefix(data[:min(len(data), interpLineMax)], []byte("#!"))
	if !ok {
		return "", "", false
	}
	if end := bytes.IndexByte(line, '\n'); end >= 0 {
		line = line[:end]
	}

	// Split the interpreter from the rest, which is one argument.
	fields := strings.TrimSpace(string(line))
	end := strings.IndexAny(fields, " \t")
	if end < 0 {
		return fields, "", fields != ""
	}
	return fields[:end], strings.TrimSpace(fields[end:]), true
}

// execChild registers the process that runs the program at exe under p's PID
// as p.selfExecProc. It keeps p's process state, such as its directory and
// credentials, and takes env when it is set. It moves p's descriptors to the
// program and closes the close-on-exec ones at once, since p lives on until
// the program exits.
func (p *Process) execChild(args []string, env map[string]string, exe string) {
	// Keep the process state with the new arguments and environment.
	child := newProcess(p.kernel, p.pid, args)
	child.procState = p.clone()
	child.args = args
	child.exe = exe
	child.started = p.started

	// Move the descriptors, closing the close-on-exec ones.
	for fd := range child.cloexec {
		child.fds[fd].unref()
		delete(child.fds, fd)
	}
	clear(child.cloexec)
	clear(p.fds)
	clear(p.cloexec)
	if env != nil {
		child.env = env
	}

	// Keep ITIMER_REAL, as exec does: its SIGALRM reaches p, which passes it
	// on to the program.
	child.realTimer, p.realTimer = p.realTimer, nil

	// Keep the signal mask, pending signals and ignored signals, as exec
	// does. Later signals go to the program.
	p.sigMu.Lock()
	p.selfExecProc = child
	child.sigMask = p.sigMask
	child.sigPending = p.sigPending
	for sig, act := range p.sigActions {
		if act.handler == sigIgn {
			child.sigActions[sig].handler = sigIgn
		}
	}

	// Take the signals in flight. A terminating signal that reached p during
	// the exec ends the program when it starts.
	child.sigTerm = p.sigTerm
	caught := p.sigCaught
	p.sigTerm, p.sigCaught = 0, 0
	p.sigMu.Unlock()

	// Exec resets the handlers, so a signal caught but not yet handled takes
	// its default action in the program, as on Linux.
	for caught != 0 {
		sig := uint32(bits.TrailingZeros64(caught)) + 1
		caught &^= sigBit(sig)
		child.deliverSignal(sig)
	}

	// Register the program under its PID.
	p.kernel.procs.set(child.pid, child)
}

// sysWait4 waits for a child to exit, writes its wait status, and reaps it.
// A pid above zero waits for that child, -1 for any child, 0 for any child in
// the caller's process group, and below -1 for any child in group -pid. With
// WNOHANG it returns zero while the children run.
func (p *Process) sysWait4(ctx context.Context, pid int32, statusPtr, options, rusagePtr uint32) uint64 {
	// Wait for a matching child to exit. A caught signal interrupts the wait.
	match := func(c *Process) bool {
		switch {
		case pid > 0:
			return c.pid == int(pid)
		case pid == 0:
			return c.pgid == p.pgid
		case pid < -1:
			return c.pgid == int(-pid)
		}
		return true
	}
	ctx, release := p.interruptible(ctx)
	defer release()
	child, err := p.kernel.procs.waitChild(ctx, p.pid, match, options&wnohang != 0)
	switch {
	case errors.Is(err, errNoChild):
		return negErrno(ECHILD)
	case err != nil:
		return negErrno(EINTR)
	case child == nil:
		return 0
	}

	// Report the exit in the Linux status encoding, the signal number for a
	// signal death or else the exit code in bits 8 to 15, with empty resource
	// usage.
	mem := p.inst.Memory()
	if statusPtr != 0 {
		status := uint32(child.exitCode&0xff) << 8
		if child.exitSignal != 0 {
			status = child.exitSignal
		}
		mem.WriteUint32Le(statusPtr, status)
	}
	if rusagePtr != 0 {
		mem.Write(rusagePtr, make([]byte, rusageSize))
	}

	// Reap it.
	p.kernel.procs.remove(child.pid)
	return uint64(uint32(child.pid))
}

// rusageSize is the size of a wasm32 struct rusage.
const rusageSize = 72

// posixExecvp runs a program found by name in the PATH directories, or at
// file when file contains a path.
func (p *Process) posixExecvp(ctx context.Context, file string, args []string) uint64 {
	path := p.lookPath(file)
	if path == "" {
		return negErrno(ENOENT)
	}
	return p.exec(ctx, path, args, nil)
}

// lookPath returns file when file contains a path, and otherwise the first
// PATH directory entry named file, or "" when there is none. An empty PATH
// entry means the working directory.
func (p *Process) lookPath(file string) string {
	if strings.Contains(file, "/") {
		return file
	}
	for dir := range strings.SplitSeq(cmp.Or(p.env["PATH"], "/bin:/usr/bin"), ":") {
		path := cmp.Or(dir, ".") + "/" + file
		if p.kernel.vfs.lookup(resolvePath(p.cwd, p.fds, atFDCWD, path)) != nil {
			return path
		}
	}
	return ""
}

// posixExit ends the program with code.
func (p *Process) posixExit(code uint32) {
	panic(&ProcessExitError{Code: int(code)})
}

// readExeclArgs reads the argument list of execl: arg0, then the variadic
// arguments at va up to a null pointer. It returns the address after the null,
// where execle passes envp.
func readExeclArgs(mem engine.Memory, arg0, va uint32) ([]string, uint32) {
	// Read the pointers up to the null.
	if arg0 == 0 {
		return nil, va
	}
	args := []string{readCString(mem, arg0)}
	for {
		ptr, ok := mem.ReadUint32Le(va)
		va += 4
		if !ok || ptr == 0 {
			return args, va
		}
		args = append(args, readCString(mem, ptr))
	}
}

// readArgvFromMemory reads a null-terminated argv array from guest memory.
func readArgvFromMemory(mem engine.Memory, ptr uint32) []string {
	if ptr == 0 {
		return nil
	}
	var args []string
	for i := uint32(0); ; i++ {
		argPtr, ok := mem.ReadUint32Le(ptr + i*4)
		if !ok || argPtr == 0 {
			break
		}
		args = append(args, readCString(mem, argPtr))
	}
	return args
}

// readEnvpFromMemory reads a null-terminated envp array from guest memory
// into a map.
func readEnvpFromMemory(mem engine.Memory, ptr uint32) map[string]string {
	if ptr == 0 {
		return nil
	}
	env := make(map[string]string)
	for i := uint32(0); ; i++ {
		envPtr, ok := mem.ReadUint32Le(ptr + i*4)
		if !ok || envPtr == 0 {
			break
		}
		if key, value, ok := strings.Cut(readCString(mem, envPtr), "="); ok {
			env[key] = value
		}
	}
	return env
}
