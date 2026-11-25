//go:build linux

package boot

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Restart and stop bounds for a supervised console child.
var (
	// childRetryMin is the first restart delay; each failure doubles it.
	childRetryMin = 2 * time.Second
	// childRetryMax caps the delay and is the run that resets it.
	childRetryMax = time.Minute
	// childStopTimeout bounds the wait after SIGTERM before containment.
	childStopTimeout = 10 * time.Second
)

// guestChild supervises a long-running console child beside the shell: the
// image's daemon, or the child that stages the next release. The console's
// single Wait4 authority reaps it and reports each exit; the child never
// waits for itself. It restarts only while the console runs. Its console
// markers read SPACEWAVE_OS_<name>_STARTED, _EXIT and so on.
type guestChild struct {
	// name is the child's name in its console markers, such as DAEMON.
	name string
	// executable is init's bootstrap binary.
	executable string
	// args follow the executable, such as enter-daemon and its root.
	args []string
	// tty receives the child's output beside the shell's.
	tty *os.File
	// pid is the running child, or zero.
	pid int
	// started is when the running child started.
	started time.Time
	// wait is the next restart delay.
	wait time.Duration
	// restart fires the scheduled restart; nil while none is scheduled.
	restart *time.Timer
}

// start execs the child through the bootstrap binary, which releases the
// vfork before touching FUSE. A failed start schedules a restart.
func (g *guestChild) start() {
	// A new session keeps the shell's terminal signals away from the child.
	process, err := os.StartProcess(g.executable, append([]string{g.executable}, g.args...), &os.ProcAttr{
		Dir: "/", Files: []*os.File{g.tty, g.tty, g.tty},
		Sys: &syscall.SysProcAttr{Setsid: true},
	})
	if err != nil {
		println("SPACEWAVE_OS_" + g.name + "_START_FAILED " + err.Error())
		g.schedule()
		return
	}

	// The console's Wait4 loop owns the reap; release only Go's handle.
	g.pid, g.started = process.Pid, time.Now()
	_ = process.Release()
	println("SPACEWAVE_OS_" + g.name + "_STARTED pid=" + strconv.Itoa(g.pid))
}

// exited records the reaped child and schedules its restart. A child that
// ran past the longest delay restarts at the shortest one.
func (g *guestChild) exited(status unix.WaitStatus) {
	println("SPACEWAVE_OS_" + g.name + "_EXIT status=" + strconv.Itoa(int(status)))
	if time.Since(g.started) > childRetryMax {
		g.wait = 0
	}
	g.pid = 0
	g.schedule()
}

// schedule arms the restart timer with the next doubled delay.
func (g *guestChild) schedule() {
	g.wait = min(max(2*g.wait, childRetryMin), childRetryMax)
	g.restart = time.NewTimer(g.wait) //gostyle:timer backoff
}

// restartC fires when a scheduled restart is due; nil blocks forever.
func (g *guestChild) restartC() <-chan time.Time {
	if g == nil || g.restart == nil {
		return nil
	}
	return g.restart.C
}

// restartDue starts the child whose restart timer fired.
func (g *guestChild) restartDue() {
	g.restart = nil
	g.start()
}

// idle reports whether the child has not started: it neither runs nor waits
// to restart.
func (g *guestChild) idle() bool {
	return g.pid == 0 && g.restart == nil
}

// stop cancels a pending restart, sends a running child SIGTERM and waits a
// bounded time for it, reaping any other exit on the way. Containment follows
// and kills whatever remains, so a child that ignores SIGTERM costs only the
// timeout.
func (g *guestChild) stop(children <-chan os.Signal) {
	// A child that is not running needs only its restart cancelled.
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
	println("Stopping the " + strings.ToLower(g.name) + ".")

	// Reap on each SIGCHLD until the child exits or the deadline passes.
	deadline := time.NewTimer(childStopTimeout) //gostyle:timer deadline
	defer deadline.Stop()
	for {
		for {
			var status unix.WaitStatus
			pid, err := unix.Wait4(-1, &status, unix.WNOHANG, nil)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				println("SPACEWAVE_OS_" + g.name + "_STOP_FAILED " + err.Error())
				return
			}
			if pid == 0 {
				break
			}
			if pid == g.pid {
				println("SPACEWAVE_OS_" + g.name + "_EXIT status=" + strconv.Itoa(int(status)))
				g.pid = 0
				return
			}
		}
		select {
		case <-children:
		case <-deadline.C:
			println("SPACEWAVE_OS_" + g.name + "_STOP_TIMEOUT")
			return
		}
	}
}
