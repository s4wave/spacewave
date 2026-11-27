//go:build linux

package boot

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// trialDeadline bounds how long a trial may take to reach health before the
// console reboots, which boots the accepted revision. It is a known expiry,
// five minutes until a measured boot sets it.
var trialDeadline = 5 * time.Minute

// daemonReady names the file the daemon rewrites in its state directory once
// its listener serves, as core/daemon's PublishReady describes.
const daemonReady = "spacewave.sock.ready"

// Stop is how a console run ends.
type Stop uint8

const (
	// PowerOff stops the machine: BusyBox poweroff, or the shell's exit.
	PowerOff Stop = iota
	// Reboot restarts the machine: BusyBox reboot, or a trial that missed its
	// deadline or failed.
	Reboot
)

// trialRun watches a trial boot from its selection: once healthy closes, a
// child process accepts the trial, and the machine reboots when the deadline
// passes before that child exits cleanly. A nil trialRun, or one already
// settled, blocks both channels.
type trialRun struct {
	// healthy closes once the trial's revision works; nil until watchReady.
	healthy <-chan struct{}
	// deadline runs expire once the trial has run out of time.
	deadline *time.Timer
	// supervised is set once the console's loop receives from expired.
	supervised atomic.Bool
	// expired closes when the deadline passes under the console's loop.
	expired chan struct{}
	// accept follows init's executable in the child that accepts the trial.
	accept []string
	// acceptPid is the running accept child, or zero.
	acceptPid int
	// settled is set once the trial is accepted.
	settled bool
}

// newTrialRun arms a trial's deadline at selection, so every boot step after
// selection counts against it, and accepts with the accept arguments.
func newTrialRun(accept []string) *trialRun {
	t := &trialRun{accept: accept, expired: make(chan struct{})}
	t.deadline = time.AfterFunc(trialDeadline, t.expire) //gostyle:timer deadline
	println("SPACEWAVE_OS_TRIAL_DEADLINE seconds=" + strconv.Itoa(int(trialDeadline.Seconds())))
	return t
}

// expire ends a trial past its deadline. Under the console's loop it asks for
// the ordered reboot. Before that a boot step may never return, so it syncs
// and restarts the machine at once, as a power cut would; every store
// survives a power cut, and the next boot rolls back.
func (t *trialRun) expire() {
	if t.supervised.Load() {
		close(t.expired)
		return
	}
	println("SPACEWAVE_OS_TRIAL_DEADLINE_PASSED")
	unix.Sync()
	_ = unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
}

// watchReady watches trial health and returns the daemon readiness event for
// staging, and a function that ends the watch. An
// image without a daemon, whose state is "", is ready once the console is,
// so its channel is already closed.
func (t *trialRun) watchReady(state string) (<-chan struct{}, func(), error) {
	// Share the same readiness event with staging even when no trial is running.
	ready, stop, err := watchReady(state)
	if err == nil && t != nil {
		t.healthy = ready
	}
	return ready, stop, err
}

// watchReady observes daemon readiness without requiring a trial.
func watchReady(state string) (<-chan struct{}, func(), error) {
	if state == "" {
		ready := make(chan struct{})
		close(ready)
		return ready, func() {}, nil
	}
	return watchWritten(state, daemonReady)
}

// supervise hands the deadline to the console's loop, which reboots in order.
func (t *trialRun) supervise() {
	if t != nil {
		t.supervised.Store(true)
	}
}

// unsettled reports whether a trial is still running, so a failure must
// reboot to roll it back.
func (t *trialRun) unsettled() bool {
	return t != nil && !t.settled
}

// healthyC returns the channel that closes once the trial works.
func (t *trialRun) healthyC() <-chan struct{} {
	if t == nil {
		return nil
	}
	return t.healthy
}

// deadlineC returns the channel that closes when the trial runs out of time
// under the console's loop.
func (t *trialRun) deadlineC() <-chan struct{} {
	if !t.unsettled() {
		return nil
	}
	return t.expired
}

// startAccept starts the child that accepts the healthy trial. The console's
// Wait4 loop reaps it; the deadline stays armed until it exits, so an accept
// that hangs still reboots.
func (t *trialRun) startAccept(executable string, tty *os.File) error {
	// Health is reached once; stop waiting for it.
	t.healthy = nil

	// The child writes the accepted revision; release only Go's handle.
	process, err := os.StartProcess(executable, append([]string{executable}, t.accept...), &os.ProcAttr{
		Dir: "/", Files: []*os.File{tty, tty, tty},
	})
	if err != nil {
		return err
	}
	t.acceptPid = process.Pid
	return process.Release()
}

// accepting reports whether pid is the running accept child.
func (t *trialRun) accepting(pid int) bool {
	return t != nil && t.acceptPid != 0 && pid == t.acceptPid
}

// accepted settles the trial when its accept child exits: a clean exit ends
// the deadline, and any other exit fails the trial.
func (t *trialRun) accepted(status unix.WaitStatus) error {
	// Only a clean exit committed the acceptance.
	t.acceptPid = 0
	if !status.Exited() || status.ExitStatus() != 0 {
		return errors.New("accept exited unsuccessfully; wait status=" + strconv.Itoa(int(status)))
	}
	t.close()
	t.settled = true
	return nil
}

// close ends the deadline; a nil trialRun has none.
func (t *trialRun) close() {
	if t == nil {
		return
	}
	if t.deadline != nil {
		t.deadline.Stop()
		t.deadline = nil
	}
}

// watchWritten returns a channel that closes once a writer closes name in dir,
// and a function that ends the watch. The daemon rewrites its readiness file
// on every start, so a file left by an earlier boot never closes the channel.
func watchWritten(dir, name string) (<-chan struct{}, func(), error) {
	// A non-blocking inotify descriptor lets Close wake the reader.
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), "inotify")
	if _, err := unix.InotifyAddWatch(fd, dir, unix.IN_CLOSE_WRITE|unix.IN_MOVED_TO); err != nil {
		return nil, nil, errors.Join(err, file.Close())
	}

	// Close written on the first event for name; a closed descriptor ends
	// the reader without closing it.
	written := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := file.Read(buf)
			if err != nil {
				return
			}
			for off := 0; off+unix.SizeofInotifyEvent <= n; {
				event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
				start := off + unix.SizeofInotifyEvent
				off = start + int(event.Len)
				if strings.TrimRight(string(buf[start:off]), "\x00") == name {
					close(written)
					return
				}
			}
		}
	}()
	return written, func() { _ = file.Close() }, nil
}
