//go:build linux

package boot

import (
	"os"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Restart and stop bounds for the image's daemon.
var (
	// daemonRetryMin is the first restart delay; each crash doubles it.
	daemonRetryMin = 2 * time.Second
	// daemonRetryMax caps the delay and is the run that resets it.
	daemonRetryMax = time.Minute
	// daemonStopTimeout bounds the wait after SIGTERM before containment.
	daemonStopTimeout = 10 * time.Second
)

// guestDaemon supervises the image's daemon as a second console child. The
// console's single Wait4 authority reaps it and reports each exit; the daemon
// never waits for itself. It restarts only while the console runs.
type guestDaemon struct {
	// executable is init's bootstrap binary, which execs enter-daemon.
	executable string
	// args follow the executable: enter-daemon, the root, and the grant flag.
	args []string
	// tty receives the daemon's output beside the shell's.
	tty *os.File
	// pid is the running daemon, or zero.
	pid int
	// started is when the running daemon started.
	started time.Time
	// wait is the next restart delay.
	wait time.Duration
	// restart fires the scheduled restart; nil while none is scheduled.
	restart *time.Timer
}

// start execs a new daemon through the bootstrap binary, which releases the
// vfork before touching FUSE. A failed start schedules a restart.
func (g *guestDaemon) start() {
	// A new session keeps the shell's terminal signals away from the daemon.
	process, err := os.StartProcess(g.executable, append([]string{g.executable}, g.args...), &os.ProcAttr{
		Dir: "/", Files: []*os.File{g.tty, g.tty, g.tty},
		Sys: &syscall.SysProcAttr{Setsid: true},
	})
	if err != nil {
		println("SPACEWAVE_OS_DAEMON_START_FAILED " + err.Error())
		g.schedule()
		return
	}

	// The console's Wait4 loop owns the reap; release only Go's handle.
	g.pid, g.started = process.Pid, time.Now()
	_ = process.Release()
	println("SPACEWAVE_OS_DAEMON_STARTED pid=" + strconv.Itoa(g.pid))
}

// exited records the reaped daemon and schedules its restart. A daemon that
// ran past the longest delay restarts at the shortest one.
func (g *guestDaemon) exited(status unix.WaitStatus) {
	println("SPACEWAVE_OS_DAEMON_EXIT status=" + strconv.Itoa(int(status)))
	if time.Since(g.started) > daemonRetryMax {
		g.wait = 0
	}
	g.pid = 0
	g.schedule()
}

// schedule arms the restart timer with the next doubled delay.
func (g *guestDaemon) schedule() {
	g.wait = min(max(2*g.wait, daemonRetryMin), daemonRetryMax)
	g.restart = time.NewTimer(g.wait) //gostyle:timer backoff
}

// restartC fires when a scheduled restart is due; nil blocks forever.
func (g *guestDaemon) restartC() <-chan time.Time {
	if g == nil || g.restart == nil {
		return nil
	}
	return g.restart.C
}

// restartDue starts the daemon whose restart timer fired.
func (g *guestDaemon) restartDue() {
	g.restart = nil
	g.start()
}

// stop cancels a pending restart, sends a running daemon SIGTERM and waits a
// bounded time for it, reaping any other exit on the way. Containment follows
// and kills whatever remains, so a daemon that ignores SIGTERM costs only the
// timeout.
func (g *guestDaemon) stop(children <-chan os.Signal) {
	// A daemon that is not running needs only its restart cancelled.
	if g == nil {
		return
	}
	if g.restart != nil {
		g.restart.Stop()
		g.restart = nil
	}
	if g.pid == 0 || unix.Kill(g.pid, unix.SIGTERM) != nil {
		return
	}
	println("Stopping the daemon.")

	// Reap on each SIGCHLD until the daemon exits or the deadline passes.
	deadline := time.NewTimer(daemonStopTimeout) //gostyle:timer deadline
	defer deadline.Stop()
	for {
		for {
			var status unix.WaitStatus
			pid, err := unix.Wait4(-1, &status, unix.WNOHANG, nil)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				println("SPACEWAVE_OS_DAEMON_STOP_FAILED " + err.Error())
				return
			}
			if pid == 0 {
				break
			}
			if pid == g.pid {
				println("SPACEWAVE_OS_DAEMON_EXIT status=" + strconv.Itoa(int(status)))
				g.pid = 0
				return
			}
		}
		select {
		case <-children:
		case <-deadline.C:
			println("SPACEWAVE_OS_DAEMON_STOP_TIMEOUT")
			return
		}
	}
}
