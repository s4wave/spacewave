package s4db

import (
	"golang.org/x/sys/unix"
)

// flushDurable makes earlier writes durable on the drive.
func (f *osFile) flushDurable() error {
	return unix.Fdatasync(int(f.Fd()))
}

// bgDirtyMax is the most a background job writes between pushes of its bytes
// to the drive: one chunk, since a push names its chunks and costs no drive
// cache flush. A commit's durable flush waits behind the push in flight, and
// under a merge forge's SATA drive took idle-class writes at about 90 MB/s, so
// a push of 8 MiB held a commit's flush for up to 93 ms.
const bgDirtyMax = chunkSize

// writeback writes the chunks' dirty pages and waits for them, without the
// drive cache flush and metadata write of Fdatasync. It names the chunks
// rather than the whole file, which would also take a commit's log pages and
// send them at the idle thread's priority while the commit waits for them.
func (f *osFile) writeback(chunks []uint64) error {
	// Write each span of adjacent chunks as one range.
	const flags = unix.SYNC_FILE_RANGE_WAIT_BEFORE | unix.SYNC_FILE_RANGE_WRITE | unix.SYNC_FILE_RANGE_WAIT_AFTER
	for i := 0; i < len(chunks); {
		j := i + 1
		for j < len(chunks) && chunks[j] == chunks[j-1]+1 {
			j++
		}
		if err := unix.SyncFileRange(int(f.Fd()), fileOff(chunkStart(chunks[i])), int64(j-i)*chunkSize, flags); err != nil {
			return err
		}
		i = j
	}
	return nil
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
