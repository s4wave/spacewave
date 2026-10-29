package s4db

import (
	"golang.org/x/sys/unix"
)

// Linux open file description locks belong to the descriptor, so a process
// sees its own other handles as conflicts and closing an unrelated
// descriptor keeps them.
const (
	// cmdSetLock takes a lock or fails.
	cmdSetLock = unix.F_OFD_SETLK
	// cmdSetLockWait takes a lock, waiting for conflicting holders.
	cmdSetLockWait = unix.F_OFD_SETLKW
	// cmdGetLock reports a conflicting lock.
	cmdGetLock = unix.F_OFD_GETLK
)

// flushDurable makes earlier writes durable on the drive.
func (f *osFile) flushDurable() error {
	return unix.Fdatasync(int(f.Fd()))
}

// flushOrdered orders earlier writes before later ones. Linux has no write
// barrier short of a full flush, so ordered commits rely on record
// checksums: recovery keeps the longest valid prefix of the log.
func (*osFile) flushOrdered() error {
	return nil
}

// flushBarrier orders earlier writes before later ones. Linux has no
// cheaper ordering than a full flush, so the barrier is durable.
func (f *osFile) flushBarrier() (bool, error) {
	return true, f.flushDurable()
}

// punch deallocates n bytes at off, keeping the file length.
func (f *osFile) punch(off, n int64) error {
	return unix.Fallocate(int(f.Fd()), unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE, off, n)
}

// ioprio_set's arguments, which x/sys/unix leaves undefined.
const (
	// ioprioWhoProcess names a thread by its id.
	ioprioWhoProcess = 1
	// ioprioIdle is the idle disk class, IOPRIO_CLASS_IDLE, shifted into
	// place.
	ioprioIdle = 3 << 13
)

// lowerThreadPriority moves the calling thread to the idle CPU policy and
// the idle disk class, so it runs only when nothing else wants the CPU or
// the disk. The disk class applies on schedulers that honor it, such as BFQ.
func lowerThreadPriority() error {
	// Take the idle CPU policy.
	attr := unix.SchedAttr{Size: unix.SizeofSchedAttr, Policy: unix.SCHED_IDLE}
	if err := unix.SchedSetAttr(0, &attr, 0); err != nil {
		return err
	}

	// Take the idle disk class.
	_, _, errno := unix.Syscall(unix.SYS_IOPRIO_SET, ioprioWhoProcess, uintptr(unix.Gettid()), ioprioIdle)
	if errno != 0 {
		return errno
	}
	return nil
}
