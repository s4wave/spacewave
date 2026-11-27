package kernel

import (
	"context"
	"math/bits"
	"time"

	"github.com/pkg/errors"
)

// errInterrupted ends a blocking call so a caught signal's handler can run.
var errInterrupted = errors.New("interrupted by signal")

// sigState is the signal state of a process. The signal sets use the Linux
// layout: bit signo-1.
type sigState struct {
	// sigActions holds each signal's disposition.
	sigActions [32]sigAction
	// sigCaught holds the delivered signals whose guest handlers have not
	// run yet.
	sigCaught uint64
	// sigMask holds the blocked signals.
	sigMask uint64
	// sigPending holds the signals delivered while blocked.
	sigPending uint64
	// altStack is the alternate signal stack sigaltstack registered.
	altStack sigAltStack
}

// sigAction is the disposition of one signal.
type sigAction struct {
	// handler is sigDfl, sigIgn, or the guest handler's table index.
	handler uint32
	flags   uint32
	// mask holds the signals blocked while the handler runs.
	mask uint64
}

// sigAltStack is an alternate signal stack. ssFlags holds ssDisable when no
// stack is installed.
type sigAltStack struct {
	ssSp    uint32
	ssFlags uint32
	ssSize  uint32
}

// sigBit returns the sigset bit for a signal number in Linux layout
// (bit signo-1).
func sigBit(sig uint32) uint64 {
	return uint64(1) << (sig - 1)
}

// wakeSigWaitersLocked wakes any rt_sigtimedwait/rt_sigsuspend waiter by
// closing and clearing the current wait channel. Callers must hold sigMu.
func (p *Process) wakeSigWaitersLocked() {
	if p.sigWaitCh != nil {
		close(p.sigWaitCh)
		p.sigWaitCh = nil
	}
}

// sigWaitChLocked returns the current signal wait channel, creating it
// lazily. Callers must hold sigMu and must read the mask/pending state they
// wait on under the same lock to avoid missed wakeups.
func (p *Process) sigWaitChLocked() chan struct{} {
	if p.sigWaitCh == nil {
		p.sigWaitCh = make(chan struct{})
	}
	return p.sigWaitCh
}

// deliverSignal delivers sig to the process. A signal blocked by the mask,
// other than SIGKILL and SIGSTOP, is held pending and wakes rt_sigtimedwait
// waiters. An unblocked signal counts as an unblocked delivery for
// rt_sigsuspend and takes its disposition: a guest handler is marked caught
// and interrupts the blocking call in progress, and the default action of a
// terminating signal cancels the process.
func (p *Process) deliverSignal(sig uint32) {
	// Ignore signal 0 and numbers past the signal sets.
	if sig == 0 || sig >= 32 {
		return
	}

	// Hand a signal that arrives after exec to the program that replaced p.
	bit := sigBit(sig)
	p.sigMu.Lock()
	if next := p.selfExecProc; next != nil {
		p.sigMu.Unlock()
		next.deliverSignal(sig)
		return
	}

	// Hold a blocked signal pending.
	if p.sigMask&bit&^sigKillStopMask != 0 {
		p.sigPending |= bit
		p.wakeSigWaitersLocked()
		p.sigMu.Unlock()
		return
	}

	// Count the delivery for rt_sigsuspend and take the disposition. SIGKILL
	// and SIGSTOP always take the default disposition.
	p.sigUnblockedDeliveries++
	p.wakeSigWaitersLocked()
	handler := p.sigActions[sig].handler
	if bit&sigKillStopMask != 0 {
		handler = sigDfl
	}
	terminate := handler == sigDfl && bit&sigTerminateMask != 0

	// Mark a signal with a guest handler caught and interrupt the blocking
	// call.
	switch {
	case handler > sigIgn:
		p.sigCaught |= bit
		if p.sigInterrupt != nil {
			p.sigInterrupt(errInterrupted)
		}
	case terminate:
		p.sigTerm = sig
	}
	cancel := p.cancel
	p.sigMu.Unlock()

	// Run the default action. A process that has not started yet ends when
	// it starts.
	if terminate && cancel != nil {
		cancel()
	}
}

// setSigAction installs act as the disposition of signum when act is not nil
// and returns the previous disposition.
func (p *Process) setSigAction(signum uint32, act *sigAction) sigAction {
	// Swap the disposition.
	p.sigMu.Lock()
	defer p.sigMu.Unlock()
	old := p.sigActions[signum]
	if act != nil {
		p.sigActions[signum] = *act
	}
	return old
}

// interruptible returns ctx extended to end with errInterrupted when a
// signal is caught, so a blocking call returns EINTR and the handler runs.
// The caller must call release when the call returns.
func (p *Process) interruptible(ctx context.Context) (ictx context.Context, release func()) {
	// Register the cancel for deliverSignal, interrupting at once when a
	// signal is already caught.
	ictx, cancel := context.WithCancelCause(ctx)
	p.sigMu.Lock()
	p.sigInterrupt = cancel
	if p.sigCaught != 0 {
		cancel(errInterrupted)
	}
	p.sigMu.Unlock()

	// Unregister it when the call returns.
	return ictx, func() {
		p.sigMu.Lock()
		p.sigInterrupt = nil
		p.sigMu.Unlock()
		cancel(nil)
	}
}

// interrupted reports whether ctx from interruptible ended for a caught
// signal.
func interrupted(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), errInterrupted)
}

// runCaughtSignals runs the guest handler of each caught signal through the
// signal trampoline. It runs after a host call returns, so the guest is at a
// safe point. A handler may longjmp, which unwinds through this call.
func (p *Process) runCaughtSignals(ctx context.Context) error {
	// Run the handlers until no signal is caught.
	trampoline := p.inst.ExportedFunction(guestSignalTrampoline)
	for {
		// Take the lowest caught signal and its handler.
		p.sigMu.Lock()
		caught := p.sigCaught
		if caught == 0 || trampoline == nil {
			p.sigMu.Unlock()
			return nil
		}
		sig := uint32(bits.TrailingZeros64(caught)) + 1
		p.sigCaught &^= sigBit(sig)
		handler := p.sigActions[sig].handler
		p.sigMu.Unlock()

		// Run the handler unless the guest reset it in the meantime.
		if handler <= sigIgn {
			continue
		}
		p.handlerDepth++
		_, err := trampoline.Call(ctx, uint64(handler), uint64(sig))
		p.handlerDepth--
		if err != nil {
			return err
		}
	}
}

// setSigMask applies an rt_sigprocmask operation with set and returns the
// previous mask. A nil set leaves the mask unchanged. SIGKILL and SIGSTOP
// are never blocked.
func (p *Process) setSigMask(how uint32, set *uint64) uint64 {
	// Change the mask.
	p.sigMu.Lock()
	old := p.sigMask
	if set != nil {
		switch how {
		case sigBlock:
			p.sigMask |= *set
		case sigUnblock:
			p.sigMask &^= *set
		case sigSetmask:
			p.sigMask = *set
		}
		p.sigMask &^= sigKillStopMask
	}
	p.sigMu.Unlock()

	// Deliver the pending signals it unblocked.
	if set != nil {
		p.flushUnblockedPending()
	}
	return old
}

// flushUnblockedPending delivers each pending signal the mask no longer
// blocks.
func (p *Process) flushUnblockedPending() {
	// Take the deliverable signals off the pending set.
	p.sigMu.Lock()
	deliverable := p.sigPending &^ p.sigMask
	p.sigPending &^= deliverable
	p.sigMu.Unlock()

	// Deliver them, lowest first.
	for deliverable != 0 {
		sig := uint32(bits.TrailingZeros64(deliverable)) + 1
		deliverable &^= sigBit(sig)
		p.deliverSignal(sig)
	}
}

// sysRtSigprocmask changes the blocked signals with the set at setPtr,
// unless setPtr is null, and writes the previous mask to oldSetPtr, unless
// it is null.
func (p *Process) sysRtSigprocmask(how, setPtr, oldSetPtr uint32) uint64 {
	// Read the set.
	mem := p.inst.Memory()
	var set *uint64
	if setPtr != 0 {
		v, ok := mem.ReadUint64Le(setPtr)
		if !ok {
			return negErrno(EFAULT)
		}
		if how != sigBlock && how != sigUnblock && how != sigSetmask {
			return negErrno(EINVAL)
		}
		set = &v
	}

	// Apply it and report the previous mask.
	old := p.setSigMask(how, set)
	if oldSetPtr != 0 && !mem.WriteUint64Le(oldSetPtr, old) {
		return negErrno(EFAULT)
	}
	return 0
}

// sysRtSigpending writes the signals held pending by the mask to setPtr.
func (p *Process) sysRtSigpending(setPtr uint32) uint64 {
	// Read the pending set.
	if setPtr == 0 {
		return 0
	}
	p.sigMu.Lock()
	pending := p.sigPending
	p.sigMu.Unlock()

	// Write it.
	if !p.inst.Memory().WriteUint64Le(setPtr, pending) {
		return negErrno(EFAULT)
	}
	return 0
}

// sysSigaltstack writes the alternate signal stack to oldSsPtr and installs
// the stack_t at ssPtr, skipping either pointer that is null.
func (p *Process) sysSigaltstack(ssPtr, oldSsPtr uint32) uint64 {
	// Report the current stack.
	mem := p.inst.Memory()
	if oldSsPtr != 0 {
		old := p.altStack
		if old.ssSp == 0 && old.ssSize == 0 {
			old.ssFlags = ssDisable
		}
		if !mem.WriteUint32Le(oldSsPtr, old.ssSp) ||
			!mem.WriteUint32Le(oldSsPtr+4, old.ssFlags) ||
			!mem.WriteUint32Le(oldSsPtr+8, old.ssSize) {
			return negErrno(EFAULT)
		}
	}
	if ssPtr == 0 {
		return 0
	}

	// Install the new stack, or none for SS_DISABLE.
	sp, ok1 := mem.ReadUint32Le(ssPtr)
	flags, ok2 := mem.ReadUint32Le(ssPtr + 4)
	size, ok3 := mem.ReadUint32Le(ssPtr + 8)
	if !ok1 || !ok2 || !ok3 {
		return negErrno(EFAULT)
	}
	p.altStack = sigAltStack{ssSp: sp, ssFlags: flags, ssSize: size}
	if flags&ssDisable != 0 {
		p.altStack = sigAltStack{ssFlags: ssDisable}
	}
	return 0
}

// waitForSignal blocks until a signal in waitSet is pending (consuming and
// returning it), the timeout elapses (-EAGAIN), or ctx is canceled (-EINTR).
// timeout < 0 waits forever; timeout == 0 polls without blocking.
func (p *Process) waitForSignal(ctx context.Context, waitSet uint64, timeout time.Duration) uint64 {
	// Take a signal within the timeout.
	wctx := ctx
	if timeout >= 0 {
		var cancel context.CancelFunc
		wctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if sig := p.takeSignal(wctx, waitSet); sig != 0 {
		return uint64(sig)
	}

	// Report whether the timeout or ctx ended the wait.
	if timeout == 0 || ctx.Err() == nil {
		return negErrno(EAGAIN)
	}
	return negErrno(EINTR)
}

// takeSignal takes the lowest pending signal in set, waiting for one until
// ctx ends. It returns 0 when ctx ends first, so a done ctx makes a take
// that does not wait.
func (p *Process) takeSignal(ctx context.Context, set uint64) uint32 {
	for {
		// Take a pending signal when there is one.
		p.sigMu.Lock()
		if got := p.sigPending & set; got != 0 {
			sig := uint32(bits.TrailingZeros64(got)) + 1
			p.sigPending &^= sigBit(sig)
			p.sigMu.Unlock()
			return sig
		}
		ch := p.sigWaitChLocked()
		p.sigMu.Unlock()

		// Wait for the next delivery.
		if !wait(ctx, ch) {
			return 0
		}
	}
}

// sysSigtimedwait waits for a signal in the set at setPtr, for up to the
// timespec at timeoutPtr or forever when it is null, consumes it and returns
// its number. It writes the number to the siginfo at infoPtr when that is
// set. A caught signal outside the set interrupts the wait.
func (p *Process) sysSigtimedwait(ctx context.Context, setPtr, infoPtr, timeoutPtr uint32) uint64 {
	// Read the set and the timeout.
	mem := p.inst.Memory()
	set, ok := mem.ReadUint64Le(setPtr)
	if !ok {
		return negErrno(EFAULT)
	}
	timeout := time.Duration(-1)
	if timeoutPtr != 0 {
		t, errno := readTimespec(mem, timeoutPtr)
		if errno != 0 {
			return negErrno(errno)
		}
		if t.Unix() < 0 {
			return negErrno(EINVAL)
		}
		timeout = time.Duration(t.UnixNano())
	}

	// Wait, then report the signal.
	ctx, release := p.interruptible(ctx)
	defer release()
	sig := p.waitForSignal(ctx, set, timeout)
	if int32(sig) > 0 && infoPtr != 0 {
		info := make([]byte, siginfoSize)
		info[0] = byte(sig)
		if !mem.Write(infoPtr, info) {
			return negErrno(EFAULT)
		}
	}
	return sig
}

// siginfoSize is the size of a siginfo_t, whose first field is the signal
// number.
const siginfoSize = 128

// sysRtSigsuspend temporarily replaces the signal mask with newMask, waits
// for a deliverable (unblocked) signal to fire, restores the previous mask,
// and returns -EINTR per the rt_sigsuspend contract.
func (p *Process) sysRtSigsuspend(ctx context.Context, maskPtr uint32) uint64 {
	newMask, ok := p.inst.Memory().ReadUint64Le(maskPtr)
	if !ok {
		return negErrno(EFAULT)
	}
	return p.suspendWithMask(ctx, newMask)
}

// suspendWithMask runs rt_sigsuspend with mask newMask until a signal is
// delivered unblocked, that is, deliverSignal takes its disposition rather
// than holding it pending. It reads the delivery count and the wait channel
// together under sigMu, so no delivery is missed.
func (p *Process) suspendWithMask(ctx context.Context, newMask uint64) uint64 {
	// Install the temporary mask, waiting for a delivery, then restore it.
	old, startCount := p.swapSigMask(newMask)
	defer p.restoreSigMask(old)
	for {
		p.sigMu.Lock()
		delivered := p.sigUnblockedDeliveries != startCount
		ch := p.sigWaitChLocked()
		p.sigMu.Unlock()
		if delivered || !wait(ctx, ch) {
			return negErrno(EINTR)
		}
	}
}

// swapSigMask installs mask, less SIGKILL and SIGSTOP, as the signal mask
// and delivers the pending signals it unblocks. It returns the old mask and
// the count of unblocked deliveries before the swap, read together.
func (p *Process) swapSigMask(mask uint64) (old, deliveries uint64) {
	// Swap the mask, reading the count under the same lock.
	p.sigMu.Lock()
	old, deliveries = p.sigMask, p.sigUnblockedDeliveries
	p.sigMask = mask &^ sigKillStopMask
	p.sigMu.Unlock()

	// Deliver the signals the new mask lets through.
	p.flushUnblockedPending()
	return old, deliveries
}

// restoreSigMask sets the mask back to old and delivers any pending signal
// the restore leaves unblocked.
func (p *Process) restoreSigMask(old uint64) {
	p.sigMu.Lock()
	p.sigMask = old
	p.sigMu.Unlock()
	p.flushUnblockedPending()
}

// sysKill sends a signal to a process when pid is positive, to the caller's
// process group when pid is zero, to every process but init and the caller
// when pid is -1, or to process group -pid. Signal 0 only checks that a
// target exists. Init receives only the signals it handles.
func (p *Process) sysKill(pid int32, sig uint32) uint64 {
	// Find the targets.
	if sig >= 32 {
		return negErrno(EINVAL)
	}
	var targets []*Process
	switch {
	case pid > 0:
		if t := p.kernel.procs.get(int(pid)); t != nil {
			targets = append(targets, t)
		}
	case pid == -1:
		// Signal every process but init and the caller, as init's shutdown
		// expects.
		targets = p.kernel.procs.filter(func(t *Process) bool { return t.pid != initPID && t != p })
	default:
		pgid := p.pgid
		if pid < 0 {
			pgid = int(-pid)
		}
		targets = p.kernel.procs.filter(func(t *Process) bool { return t.pgid == pgid })
	}
	if len(targets) == 0 {
		return negErrno(ESRCH)
	}

	// Send the signal. Init drops a signal it has no handler for, as on
	// Linux, so a process cannot end init by accident.
	for _, t := range targets {
		if !t.initIgnores(sig) {
			t.deliverSignal(sig)
		}
	}
	return 0
}

// initIgnores reports whether p is init and keeps the default disposition of
// sig, which a signal from another process does not override. A blocked
// signal stays pending, as init may take it with sigtimedwait or install a
// handler before unblocking it.
func (p *Process) initIgnores(sig uint32) bool {
	if p.pid != initPID || sig == 0 || sig >= 32 {
		return false
	}
	p.sigMu.Lock()
	defer p.sigMu.Unlock()
	return p.sigActions[sig].handler == sigDfl && p.sigMask&^sigKillStopMask&sigBit(sig) == 0
}

// sysRtSigaction writes signum's disposition to oldActPtr and installs the
// one at actPtr, skipping either pointer that is null. The signal trampoline
// passes a handler the signal number alone, so it refuses SA_SIGINFO rather
// than call a three-argument handler with one.
func (p *Process) sysRtSigaction(signum, actPtr, oldActPtr uint32) uint64 {
	// SIGKILL and SIGSTOP keep their default action.
	if signum == 0 || signum >= 32 || signum == SIGKILL || signum == SIGSTOP {
		return negErrno(EINVAL)
	}

	// Read the new action before writing the old one, which may share its
	// memory.
	mem := p.inst.Memory()
	var act *sigAction
	if actPtr != 0 {
		handler, _ := mem.ReadUint32Le(actPtr)
		flags, _ := mem.ReadUint32Le(actPtr + 4)
		maskLo, _ := mem.ReadUint32Le(actPtr + 8)
		maskHi, _ := mem.ReadUint32Le(actPtr + 12)
		if flags&guestSaSiginfo != 0 {
			return negErrno(EINVAL)
		}
		act = &sigAction{handler: handler, flags: flags, mask: uint64(maskLo) | uint64(maskHi)<<32}
	}

	// Install it and report the old one.
	old := p.setSigAction(signum, act)
	if oldActPtr != 0 {
		mem.WriteUint32Le(oldActPtr, old.handler)
		mem.WriteUint32Le(oldActPtr+4, old.flags)
		mem.WriteUint32Le(oldActPtr+8, uint32(old.mask))
		mem.WriteUint32Le(oldActPtr+12, uint32(old.mask>>32))
	}
	return 0
}
