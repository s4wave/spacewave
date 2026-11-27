package kernel

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"maps"
	"math"
	goruntime "runtime"
	"slices"
	"time"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// wasiAtFDCWD is wasi-libc's AT_FDCWD, -2 as uint32. The WasiVM libc passes
// every path it is given unchanged with this directory descriptor, so the
// kernel resolves paths as Linux does and opens no preopened directories:
// every descriptor from 3 up is the program's.
const wasiAtFDCWD = 0xFFFFFFFE

// WASI errno values the kernel returns directly. wasiErrno translates the
// rest from Linux.
const (
	wasiErrnoSuccess  = 0
	wasiErrnoBadf     = 8
	wasiErrnoFault    = 21
	wasiErrnoInval    = 28
	wasiErrnoIo       = 29
	wasiErrnoNosys    = 52
	wasiErrnoOverflow = 61
)

// poll_oneoff event types, the subscription flag for an absolute clock
// timeout, and the sizes of a subscription and an event.
const (
	wasiEventtypeClock      = 0
	wasiEventtypeFDRead     = 1
	wasiEventtypeFDWrite    = 2
	wasiEventrwflagsHangup  = 1
	wasiSubclockAbstime     = 1
	wasiSubscriptionSize    = 48
	wasiEventSize           = 32
	wasiMaxDurationNanosecs = math.MaxInt64
)

// wasiFunctions returns the wasi_snapshot_preview1 module.
func wasiFunctions() map[string]hostFunc {
	fns := map[string]hostFunc{
		"args_get": wasiFunc("iii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return wasiWriteStrings(mem, p.args, uint32(a[0]), uint32(a[1]))
		}),
		"args_sizes_get": wasiFunc("iii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return wasiWriteStringSizes(mem, p.args, uint32(a[0]), uint32(a[1]))
		}),
		"clock_res_get": wasiFunc("iii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiClockResGet(mem, uint32(a[0]), uint32(a[1]))
		}),
		"clock_time_get": wasiFunc("iiji", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiClockTimeGet(mem, uint32(a[0]), uint32(a[2]))
		}),
		"environ_get": wasiFunc("iii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return wasiWriteStrings(mem, p.wasiEnviron(), uint32(a[0]), uint32(a[1]))
		}),
		"environ_sizes_get": wasiFunc("iii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return wasiWriteStringSizes(mem, p.wasiEnviron(), uint32(a[0]), uint32(a[1]))
		}),
		"poll_oneoff": wasiFunc("iiiii", func(ctx context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPollOneoff(ctx, mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]))
		}),
		"proc_exit": {"vi", exitFunc},
		"random_get": wasiFunc("iii", func(_ context.Context, _ *Process, mem engine.Memory, a []uint64) uint32 {
			return wasiRandomGet(mem, uint32(a[0]), uint32(a[1]))
		}),
		"sched_yield": wasiFunc("i", func(context.Context, *Process, engine.Memory, []uint64) uint32 {
			goruntime.Gosched()
			return wasiErrnoSuccess
		}),
		"sock_accept": wasiFunc("iiii", func(ctx context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiSockAccept(ctx, mem, uint32(a[0]), uint32(a[1]), uint32(a[2]))
		}),
		"sock_recv": wasiFunc("iiiiiii", func(ctx context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiSockRecv(ctx, mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[4]), uint32(a[5]))
		}),
		"sock_send": wasiFunc("iiiiii", func(ctx context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiSockSend(ctx, mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[4]))
		}),
		"sock_shutdown": wasiFunc("iii", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return wasiErrnoFromResult(p.sysShutdown(uint32(a[0]), uint32(a[1])))
		}),
	}
	maps.Copy(fns, wasiFSFunctions())
	return fns
}

// wasiFunc builds a host function from a WASI call that returns an errno.
func wasiFunc(sig string, call func(ctx context.Context, p *Process, mem engine.Memory, a []uint64) uint32) hostFunc {
	return hostFunc{sig, func(ctx context.Context, p *Process, a []uint64) uint64 {
		return uint64(call(ctx, p, p.inst.Memory(), a))
	}}
}

// wasiEnviron returns the environment as KEY=VALUE strings sorted by key.
func (p *Process) wasiEnviron() []string {
	env := make([]string, 0, len(p.env))
	for _, key := range slices.Sorted(maps.Keys(p.env)) {
		env = append(env, key+"="+p.env[key])
	}
	return env
}

// wasiClockTimeGet writes clock id's time in nanoseconds to timePtr.
func (p *Process) wasiClockTimeGet(mem engine.Memory, id, timePtr uint32) uint32 {
	t, ok := p.kernel.nowForClock(id)
	switch {
	case !ok:
		return wasiErrnoInval
	case !mem.WriteUint64Le(timePtr, uint64(t.UnixNano())):
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiClockResGet writes clock id's resolution in nanoseconds to resPtr.
func (p *Process) wasiClockResGet(mem engine.Memory, id, resPtr uint32) uint32 {
	sec, nsec, ok := clockResolution(id)
	switch {
	case !ok:
		return wasiErrnoInval
	case !mem.WriteUint64Le(resPtr, uint64(sec)*uint64(time.Second)+uint64(nsec)):
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiRandomGet fills bufLen bytes at bufPtr with random data.
func wasiRandomGet(mem engine.Memory, bufPtr, bufLen uint32) uint32 {
	buf := make([]byte, bufLen)
	_, _ = rand.Read(buf)
	if !mem.Write(bufPtr, buf) {
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiPollEvent is an event poll_oneoff reports.
type wasiPollEvent struct {
	userdata  uint64
	errno     uint16
	eventType uint8
	// nbytes is the bytes a descriptor can read or write.
	nbytes uint64
	// flags holds wasiEventrwflagsHangup when the other side closed.
	flags uint16
}

// wasiPoll is a poll_oneoff call's subscriptions.
type wasiPoll struct {
	// ready holds the events found while reading the subscriptions: clocks
	// already due.
	ready []wasiPollEvent
	// fds holds the descriptor subscriptions.
	fds []wasiPollFD
	// clock is the earliest clock event not yet due, due after wait.
	clock *wasiPollEvent
	wait  time.Duration
}

// wasiPollFD is a subscription to a descriptor's readiness.
type wasiPollFD struct {
	event wasiPollEvent
	fd    uint32
}

// wasiPollOneoff writes to outPtr the events of nsubscriptions subscriptions
// at inPtr that happen first, and their count to neventsPtr. When no event
// has happened it waits for a change on any descriptor, the clock or a
// caught signal, which ends the wait with EINTR, then checks again.
func (p *Process) wasiPollOneoff(ctx context.Context, mem engine.Memory, inPtr, outPtr, nsubscriptions, neventsPtr uint32) uint32 {
	// Read the subscriptions.
	if nsubscriptions == 0 {
		return wasiErrnoInval
	}
	poll, errno := p.wasiReadPoll(mem, inPtr, nsubscriptions)
	if errno != wasiErrnoSuccess {
		return errno
	}

	// Start the clock.
	var timerCh <-chan time.Time
	if poll.clock != nil {
		timer := time.NewTimer(poll.wait)
		defer timer.Stop()
		timerCh = timer.C
	}

	// Wait until an event happens.
	ctx, release := p.interruptible(ctx)
	defer release()
	for {
		// Report the events that happened.
		ready, changes := p.wasiPollFDs(poll)
		if len(ready) != 0 || (len(changes) == 0 && poll.clock == nil) {
			return wasiWritePollEvents(mem, ready, outPtr, neventsPtr)
		}

		// Wait for a change, the clock or a caught signal.
		switch waitAny(ctx, timerCh, changes) {
		case waitDone:
			return wasiErrno(EINTR)
		case waitTimer:
			return wasiWritePollEvents(mem, []wasiPollEvent{*poll.clock}, outPtr, neventsPtr)
		}
	}
}

// wasiPollFDs returns the events that have happened: the clocks already
// due and the descriptors that are ready. It also returns the channels that
// close at the next change of each descriptor that is not ready.
func (p *Process) wasiPollFDs(poll *wasiPoll) ([]wasiPollEvent, []<-chan struct{}) {
	ready := slices.Clone(poll.ready)
	var changes []<-chan struct{}
	for _, sub := range poll.fds {
		// Check the descriptor, which may have closed since the last check.
		event := sub.event
		isReady, hangup, change := p.fdReadiness(sub.fd, event.eventType == wasiEventtypeFDWrite)
		switch {
		case change == nil && !isReady:
			event.errno = wasiErrnoBadf
		case !isReady:
			changes = append(changes, change)
			continue
		}
		if hangup {
			event.flags = wasiEventrwflagsHangup
		}
		ready = append(ready, event)
	}
	return ready, changes
}

// fdReadiness reports whether a read of descriptor fd, or a write when write
// is set, would proceed without waiting, and whether the other side closed.
// change closes at the descriptor's next change while it is not ready. A
// descriptor that is not open reports neither readiness nor a change.
func (p *Process) fdReadiness(fd uint32, write bool) (ready, hangup bool, change <-chan struct{}) {
	of, ok := p.fds[fd]
	if !ok {
		return false, false, nil
	}
	return of.readiness(write)
}

// wasiReadPoll reads n poll_oneoff subscriptions at inPtr into the clocks
// already due, the earliest clock to wait for and the descriptors.
func (p *Process) wasiReadPoll(mem engine.Memory, inPtr, n uint32) (*wasiPoll, uint32) {
	poll := &wasiPoll{}
	for i := range n {
		// Read the subscription's header.
		subPtr := inPtr + i*wasiSubscriptionSize
		userdata, ok1 := mem.ReadUint64Le(subPtr)
		eventType, ok2 := mem.ReadByteAt(subPtr + 8)
		fd, ok3 := mem.ReadUint32Le(subPtr + 16)
		if !ok1 || !ok2 || !ok3 {
			return nil, wasiErrnoFault
		}
		event := wasiPollEvent{userdata: userdata, eventType: eventType}

		// File a clock event as ready or as the earliest wait, and a
		// descriptor event for checking.
		switch eventType {
		case wasiEventtypeClock:
			wait, errno := p.wasiPollClockWait(mem, subPtr)
			switch {
			case errno != wasiErrnoSuccess:
				return nil, errno
			case wait <= 0:
				poll.ready = append(poll.ready, event)
			case poll.clock == nil || wait < poll.wait:
				poll.clock, poll.wait = &event, wait
			}
		case wasiEventtypeFDRead, wasiEventtypeFDWrite:
			event.nbytes = 1
			poll.fds = append(poll.fds, wasiPollFD{event: event, fd: fd})
		default:
			return nil, wasiErrnoInval
		}
	}
	return poll, wasiErrnoSuccess
}

// wasiPollClockWait returns how long until the clock subscription at subPtr
// is due.
func (p *Process) wasiPollClockWait(mem engine.Memory, subPtr uint32) (time.Duration, uint32) {
	// Read the clock, the timeout and the flags.
	id, ok1 := mem.ReadUint32Le(subPtr + 16)
	timeout, ok2 := mem.ReadUint64Le(subPtr + 24)
	flags, ok3 := mem.ReadUint32Le(subPtr + 40)
	if !ok1 || !ok2 || !ok3 {
		return 0, wasiErrnoFault
	}
	if flags&wasiSubclockAbstime == 0 {
		return time.Duration(min(timeout, wasiMaxDurationNanosecs)), wasiErrnoSuccess
	}

	// Measure an absolute timeout from the clock's current time.
	var now uint64
	switch id {
	case clockRealtime:
		now = uint64(p.kernel.realtimeNow().UnixNano())
	case clockMonotonic, clockProcessCPUTimeID, clockThreadCPUTimeID:
		now = uint64(p.kernel.monotonicElapsed())
	default:
		return 0, wasiErrnoInval
	}
	if timeout <= now {
		return 0, wasiErrnoSuccess
	}
	return time.Duration(min(timeout-now, wasiMaxDurationNanosecs)), wasiErrnoSuccess
}

// wasiWritePollEvents writes events to outPtr and their count to neventsPtr.
func wasiWritePollEvents(mem engine.Memory, events []wasiPollEvent, outPtr, neventsPtr uint32) uint32 {
	for i, event := range events {
		var buf [wasiEventSize]byte
		binary.LittleEndian.PutUint64(buf[0:], event.userdata)
		binary.LittleEndian.PutUint16(buf[8:], event.errno)
		buf[10] = event.eventType
		binary.LittleEndian.PutUint64(buf[16:], event.nbytes)
		binary.LittleEndian.PutUint16(buf[24:], event.flags)
		if !mem.Write(outPtr+uint32(i)*wasiEventSize, buf[:]) {
			return wasiErrnoFault
		}
	}
	if !mem.WriteUint32Le(neventsPtr, uint32(len(events))) {
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiWriteStringSizes writes the count of values to countPtr and the bytes
// they take as NUL-terminated strings to bufSizePtr.
func wasiWriteStringSizes(mem engine.Memory, values []string, countPtr, bufSizePtr uint32) uint32 {
	// Total the strings.
	var bufSize uint64
	for _, value := range values {
		bufSize += uint64(len(value)) + 1
	}
	if bufSize > math.MaxUint32 {
		return wasiErrnoOverflow
	}

	// Write the sizes.
	if !mem.WriteUint32Le(countPtr, uint32(len(values))) || !mem.WriteUint32Le(bufSizePtr, uint32(bufSize)) {
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiWriteStrings writes values as NUL-terminated strings to bufPtr and a
// pointer to each to the array at ptrsPtr.
func wasiWriteStrings(mem engine.Memory, values []string, ptrsPtr, bufPtr uint32) uint32 {
	off := bufPtr
	for i, value := range values {
		if !mem.WriteUint32Le(ptrsPtr+uint32(i)*4, off) || !mem.Write(off, append([]byte(value), 0)) {
			return wasiErrnoFault
		}
		off += uint32(len(value) + 1)
	}
	return wasiErrnoSuccess
}

// wasiReadPath reads a path of length bytes at ptr.
func wasiReadPath(mem engine.Memory, ptr, length uint32) (string, uint32) {
	buf, ok := mem.Read(ptr, length)
	if !ok {
		return "", wasiErrnoFault
	}
	return string(buf), wasiErrnoSuccess
}

// wasiSockAccept accepts a connection on a listening socket and writes its
// descriptor to fdPtr. flags holds the WASI descriptor flags of the new
// descriptor.
func (p *Process) wasiSockAccept(ctx context.Context, mem engine.Memory, fd, flags, fdPtr uint32) uint32 {
	// Accept the connection.
	var status uint32
	if flags&wasiFdflagsNonblock != 0 {
		status = oNONBLOCK
	}
	newFD, errno := wasiResult(p.sysAccept(ctx, fd, status))

	// Return its descriptor.
	switch {
	case errno != wasiErrnoSuccess:
		return errno
	case !mem.WriteUint32Le(fdPtr, newFD):
		_ = p.sysClose(newFD)
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiSockRecv reads a connected socket into the iovecs at iovsPtr. It writes
// the bytes read to nPtr and no output flags to flagsPtr. Peeking is
// unsupported. Waiting for all the data returns what arrived.
func (p *Process) wasiSockRecv(ctx context.Context, mem engine.Memory, fd, iovsPtr, iovsLen, flags, nPtr, flagsPtr uint32) uint32 {
	// Check the socket and the flags.
	if _, errno := p.socketFile(fd); errno != 0 {
		return wasiErrnoFromResult(errno)
	}
	if flags&wasiRiflagsRecvPeek != 0 {
		return wasiErrno(EOPNOTSUPP)
	}

	// Read the data.
	errno := wasiTransfer(mem, iovsPtr, iovsLen, nPtr, p.wasiReadMove(ctx, fd))
	if errno == wasiErrnoSuccess && !mem.Write(flagsPtr, []byte{0, 0}) {
		return wasiErrnoFault
	}
	return errno
}

// wasiSockSend writes the iovecs at iovsPtr to a connected socket and the
// bytes written to nPtr.
func (p *Process) wasiSockSend(ctx context.Context, mem engine.Memory, fd, iovsPtr, iovsLen, nPtr uint32) uint32 {
	if _, errno := p.socketFile(fd); errno != 0 {
		return wasiErrnoFromResult(errno)
	}
	return wasiTransfer(mem, iovsPtr, iovsLen, nPtr, func(base, length uint32) (uint32, uint32) {
		return wasiResult(p.sysWrite(ctx, fd, base, length))
	})
}
