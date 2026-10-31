package s4db

import (
	"golang.org/x/sys/unix"
)

// flushDurable makes earlier writes durable on the drive.
func (f *osFile) flushDurable() error {
	return unix.Fdatasync(int(f.Fd()))
}

// writeback writes the file's dirty pages and waits for them, without the
// drive cache flush and metadata write of Fdatasync.
func (f *osFile) writeback() error {
	return unix.SyncFileRange(int(f.Fd()), 0, 0, unix.SYNC_FILE_RANGE_WAIT_BEFORE|unix.SYNC_FILE_RANGE_WRITE|unix.SYNC_FILE_RANGE_WAIT_AFTER)
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
