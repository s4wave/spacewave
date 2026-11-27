package kernel

import (
	"encoding/binary"
	"time"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// nowForClock returns the current time of a clock. Monotonic clocks, and the
// CPU-time clocks they stand in for, count from boot. It reports false for an
// unknown clock.
func (k *Kernel) nowForClock(clockID uint32) (time.Time, bool) {
	switch clockID {
	case clockRealtime, clockRealtimeCoarse:
		return k.realtimeNow(), true
	case clockMonotonic, clockMonotonicRaw, clockMonotonicCoarse, clockBoottime,
		clockProcessCPUTimeID, clockThreadCPUTimeID:
		return time.Unix(0, int64(k.monotonicElapsed())), true
	default:
		return time.Time{}, false
	}
}

// realtimeNow returns the host time shifted by the kernel's realtime offset.
func (k *Kernel) realtimeNow() time.Time {
	k.timeMu.Lock()
	offset := k.realtimeOffset
	k.timeMu.Unlock()
	return time.Now().Add(offset)
}

// setRealtime sets the kernel's realtime clock to target without changing
// the host clock.
func (k *Kernel) setRealtime(target time.Time) {
	k.timeMu.Lock()
	k.realtimeOffset = time.Until(target)
	k.timeMu.Unlock()
}

// monotonicElapsed returns the time since the kernel booted.
func (k *Kernel) monotonicElapsed() time.Duration {
	return time.Since(k.bootTime)
}

// clockResolution returns a clock's resolution as seconds and nanoseconds. It
// reports false for an unknown clock.
func clockResolution(clockID uint32) (uint32, uint32, bool) {
	switch clockID {
	case clockRealtimeCoarse, clockMonotonicCoarse:
		return 0, uint32(time.Millisecond), true
	case clockRealtime, clockMonotonic, clockMonotonicRaw, clockBoottime,
		clockProcessCPUTimeID, clockThreadCPUTimeID:
		return 0, 1, true
	default:
		return 0, 0, false
	}
}

// timevalSize is the size of a wasm32 struct timeval, a 64-bit tv_sec and a
// 64-bit tv_usec. A struct itimerval holds two: the interval, then the value.
const timevalSize = 16

// readTimespec reads a wasm32 struct timespec, a 64-bit tv_sec and a 32-bit
// tv_nsec. It returns an errno for an unreadable pointer or an out-of-range
// nanosecond field.
func readTimespec(mem engine.Memory, ptr uint32) (time.Time, uint32) {
	sec, ok1 := mem.ReadUint64Le(ptr)
	nsec, ok2 := mem.ReadUint32Le(ptr + 8)
	switch {
	case !ok1 || !ok2:
		return time.Time{}, EFAULT
	case nsec >= uint32(time.Second):
		return time.Time{}, EINVAL
	}
	return time.Unix(int64(sec), int64(nsec)), 0
}

// readTimeval reads a wasm32 struct timeval as a duration. It returns an
// errno for an unreadable pointer or a negative or out-of-range field.
func readTimeval(mem engine.Memory, ptr uint32) (time.Duration, uint32) {
	sec, ok1 := mem.ReadUint64Le(ptr)
	usec, ok2 := mem.ReadUint64Le(ptr + 8)
	switch {
	case !ok1 || !ok2:
		return 0, EFAULT
	case int64(sec) < 0 || usec >= uint64(time.Second/time.Microsecond):
		return 0, EINVAL
	}
	return time.Duration(sec)*time.Second + time.Duration(usec)*time.Microsecond, 0
}

// readItimerval reads an itimerval's interval and value from guest memory.
func readItimerval(mem engine.Memory, ptr uint32) (time.Duration, time.Duration, uint32) {
	// Read the interval, then the value after it.
	interval, errno := readTimeval(mem, ptr)
	if errno != 0 {
		return 0, 0, errno
	}
	value, errno := readTimeval(mem, ptr+timevalSize)
	if errno != 0 {
		return 0, 0, errno
	}
	return interval, value, 0
}

// writeItimerval writes interval and value as a wasm32 struct itimerval,
// reporting false for an unwritable pointer.
func writeItimerval(mem engine.Memory, ptr uint32, interval, value time.Duration) bool {
	buf := make([]byte, 2*timevalSize)
	for i, d := range []time.Duration{interval, value} {
		// Round up, so an armed timer never reads as disarmed.
		d = (d + time.Microsecond - 1).Truncate(time.Microsecond)
		binary.LittleEndian.PutUint64(buf[i*timevalSize:], uint64(d/time.Second))
		binary.LittleEndian.PutUint64(buf[i*timevalSize+8:], uint64(d%time.Second/time.Microsecond))
	}
	return mem.Write(ptr, buf)
}

// sysClockSettime sets the kernel's realtime clock from a timespec.
func (p *Process) sysClockSettime(clockID, tpPtr uint32) uint64 {
	// Only the realtime clock can be set.
	if clockID != clockRealtime {
		return negErrno(EINVAL)
	}
	if tpPtr == 0 {
		return negErrno(EFAULT)
	}

	// Read the time and set it.
	t, errno := readTimespec(p.inst.Memory(), tpPtr)
	if errno != 0 {
		return negErrno(errno)
	}
	p.kernel.setRealtime(t)
	return 0
}

// sysSettimeofday sets the kernel's realtime clock from a timeval, when tvPtr
// is set. Linux ignores the time zone.
func (p *Process) sysSettimeofday(tvPtr uint32) uint64 {
	// Read the time, if any, and set it.
	if tvPtr == 0 {
		return 0
	}
	d, errno := readTimeval(p.inst.Memory(), tvPtr)
	if errno != 0 {
		return negErrno(errno)
	}
	p.kernel.setRealtime(time.Unix(0, 0).Add(d))
	return 0
}

// sysGetitimer writes the time until the interval timer which next expires
// and its period. Only ITIMER_REAL runs. The CPU time timers read disarmed.
func (p *Process) sysGetitimer(which, curPtr uint32) uint64 {
	// Read the timer, then write its setting.
	if which > itimerProf {
		return negErrno(EINVAL)
	}
	var value, interval time.Duration
	if which == itimerReal && p.realTimer != nil {
		value, interval = p.realTimer.get()
	}
	if !writeItimerval(p.inst.Memory(), curPtr, interval, value) {
		return negErrno(EFAULT)
	}
	return 0
}

// sysSetitimer sets an interval timer from the itimerval at newPtr, writing
// the old setting to oldPtr when it is set. ITIMER_REAL sends SIGALRM. The
// kernel keeps no CPU time, so the CPU time timers can only be disarmed.
func (p *Process) sysSetitimer(which, newPtr, oldPtr uint32) uint64 {
	// Read the new setting.
	if which > itimerProf {
		return negErrno(EINVAL)
	}
	if newPtr == 0 {
		return negErrno(EFAULT)
	}
	mem := p.inst.Memory()
	interval, value, errno := readItimerval(mem, newPtr)
	if errno != 0 {
		return negErrno(errno)
	}

	// Apply the new setting and report the old setting.
	var oldValue, oldInterval time.Duration
	switch {
	case which == itimerReal:
		oldValue, oldInterval = p.setRealTimer(value, interval)
	case value != 0:
		return enosys
	}
	if oldPtr != 0 && !writeItimerval(mem, oldPtr, oldInterval, oldValue) {
		return negErrno(EFAULT)
	}
	return 0
}

// sysAlarm arms ITIMER_REAL to expire once after sec seconds, or disarms it
// for zero, and returns the seconds left on the old setting, rounded up.
func (p *Process) sysAlarm(sec uint32) uint64 {
	old, _ := p.setRealTimer(time.Duration(sec)*time.Second, 0)
	return uint64((old + time.Second - 1) / time.Second)
}

// setRealTimer sets ITIMER_REAL, which sends SIGALRM at each expiration, and
// returns its old setting. The timer is created on first use and lives until
// the process exits, passing to the program an exec runs.
func (p *Process) setRealTimer(value, interval time.Duration) (oldValue, oldInterval time.Duration) {
	if p.realTimer == nil {
		if value == 0 {
			return 0, 0
		}
		p.realTimer = newIntervalTimer(p.kernel, clockMonotonic, func(uint64) {
			p.deliverSignal(SIGALRM)
		}, nil)
	}
	return p.realTimer.set(value, interval, false)
}
