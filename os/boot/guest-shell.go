//go:build linux

package boot

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// runGuestShell owns every wait after mounting helpers finish. The independent
// bootstrap exec releases the parent's vfork before touching its FUSE tree.
// joined proves guest processes have stopped, including adopted descendants.
// daemonArgs, when set, start the image's daemon beside the shell. trial, when
// set, is accepted by a child once healthy, or ends the run with Reboot at its
// deadline. stageArgs, when set, start the child that stages the next release
// once ready closes and no trial is unsettled. The child's clean exit ends the
// run with Reboot, which boots the staged trial. A request ends the run with
// Reboot for SIGTERM, which BusyBox reboot sends, and with PowerOff for any
// other signal.
func runGuestShell(
	ctx context.Context,
	requests <-chan os.Signal,
	executable, root, terminal string,
	daemonArgs, stageArgs []string,
	ready <-chan struct{},
	trial *trialRun,
) (stop Stop, joined bool, retErr error) {
	// Global waits and signals are forbidden in the hosted mount-namespace probe.
	if os.Getpid() != 1 {
		return PowerOff, false, errors.New("interactive supervision requires actual guest PID1")
	}

	// Open the serial device itself: /dev/console cannot acquire a controlling tty.
	tty, err := os.OpenFile(terminal, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return PowerOff, true, err
	}
	defer func() { retErr = errors.Join(retErr, tty.Close()) }()
	if err := prepareSerial(int(tty.Fd())); err != nil {
		return PowerOff, true, err
	}

	// Subscribe before fork so even a failed trampoline exec cannot lose its wake.
	children := make(chan os.Signal, 1)
	signal.Notify(children, syscall.SIGCHLD)
	defer signal.Stop(children)

	// Exec independent pages with a new serial session before entering FUSE.
	child, err := os.StartProcess(executable, []string{executable, "enter-console", root}, &os.ProcAttr{
		Dir: "/", Env: []string{"GOMAXPROCS=1"},
		Files: []*os.File{tty, tty, tty},
		Sys:   &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0},
	})
	if err != nil {
		return PowerOff, true, err
	}
	defer func() { retErr = errors.Join(retErr, child.Release()) }()
	defer func() {
		// Kill all guest jobs, including detached sessions, then reap
		// to ECHILD. Release closes Go's process descriptor. Release
		// never calls Cmd.Wait.
		err := containGuest()
		joined = err == nil
		retErr = errors.Join(retErr, err)
	}()

	// Start the daemon after containment is deferred. Stopping the daemon
	// runs first.
	var daemon *guestChild
	if daemonArgs != nil {
		daemon = &guestChild{name: "DAEMON", executable: executable, args: daemonArgs, tty: tty}
		daemon.start()
		defer daemon.stop(children)
	}

	// The stager is the daemon's client, so it stops first. It starts at the
	// first pass with stageC set.
	var stager *guestChild
	if stageArgs != nil {
		stager = &guestChild{name: "STAGE", executable: executable, args: stageArgs, tty: tty}
		defer stager.stop(children)
	}
	stageC := func() <-chan struct{} {
		if stager == nil || !stager.idle() || trial.unsettled() {
			return nil
		}
		return ready
	}
	trial.supervise()
	println("Console ready. Save files under /data; run poweroff or sync; exit to shut down.")
	println("SPACEWAVE_OS_CONSOLE_READY pid=" + strconv.Itoa(child.Pid))

	// Drain adopted orphans on each SIGCHLD while the shell owns its own job waits.
	for {
		for {
			var status unix.WaitStatus
			pid, err := unix.Wait4(-1, &status, unix.WNOHANG, nil)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				return PowerOff, false, errors.Join(errors.New("guest child wait failed"), err)
			}
			if pid == 0 {
				break
			}
			if daemon.running(pid) {
				daemon.exited(status)
				continue
			}
			if stager.running(pid) {
				if stager.stageExited(status) {
					println("SPACEWAVE_OS_STAGED_REBOOT")
					return Reboot, false, nil
				}
				continue
			}
			if trial.accepting(pid) {
				if err := trial.accepted(status); err != nil {
					return PowerOff, false, errors.Join(errors.New("accept trial"), err)
				}
				continue
			}
			if pid == child.Pid {
				println("SPACEWAVE_OS_SHELL_EXIT status=" + strconv.Itoa(int(status)))
				if !status.Exited() || status.ExitStatus() != 0 {
					return PowerOff, false, errors.New("shell exited unsuccessfully; wait status=" + strconv.Itoa(int(status)))
				}
				return PowerOff, false, nil
			}
		}

		// Server/controller failure cancels ctx and disconnects FUSE before reaping.
		select {
		case sig := <-requests:
			if sig == syscall.SIGTERM {
				println("Reboot requested.")
				return Reboot, false, nil
			}
			println("Poweroff requested.")
			return PowerOff, false, nil
		case <-ctx.Done():
			return PowerOff, false, context.Cause(ctx)
		case <-children:
		case <-daemon.restartC():
			daemon.restartDue()
		case <-stageC():
			stager.start()
		case <-stager.restartC():
			stager.restartDue()
		case <-trial.healthyC():
			if err := trial.startAccept(executable, tty); err != nil {
				return PowerOff, false, errors.Join(errors.New("accept trial"), err)
			}
		case <-trial.deadlineC():
			println("SPACEWAVE_OS_TRIAL_DEADLINE_PASSED")
			return Reboot, false, nil
		}
	}
}

// containGuest signals only from actual PID1 and proves that every child exited.
// Repeating containment after each reap also catches a fork concurrent with the
// first broadcast. Kernel tasks are not children of this init and stay untouched.
func containGuest() error {
	// Keep this guard at the dangerous syscall, independent of the entry-point guard.
	if os.Getpid() != 1 {
		return errors.New("refusing guest-wide containment outside PID1")
	}
	println("Stopping console jobs.")

	// A stopped shell may leave detached jobs. Process-group signals miss
	// those.
	for {
		if err := unix.Kill(-1, unix.SIGKILL); err != nil && err != unix.ESRCH {
			return err
		}
		_, err := unix.Wait4(-1, nil, 0, nil)
		if err == unix.ECHILD {
			println("SPACEWAVE_OS_CHILDREN_JOINED")
			return nil
		}
		if err != nil && err != unix.EINTR {
			return err
		}
	}
}

// prepareSerial enables normal canonical input, echo and terminal-generated job
// signals without changing the serial speed configured by the kernel console.
func prepareSerial(fd int) error {
	// Preserve device settings while selecting conventional shell line discipline.
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}
	t.Iflag = (t.Iflag &^ (unix.IGNCR | unix.INLCR | unix.ISTRIP)) | unix.ICRNL
	t.Oflag |= unix.OPOST | unix.ONLCR
	t.Lflag |= unix.ISIG | unix.ICANON | unix.ECHO | unix.ECHOE | unix.ECHOK | unix.IEXTEN
	t.Cc[unix.VINTR], t.Cc[unix.VQUIT], t.Cc[unix.VSUSP] = 3, 28, 26
	t.Cc[unix.VERASE], t.Cc[unix.VKILL], t.Cc[unix.VEOF] = 127, 21, 4
	return unix.IoctlSetTermios(fd, unix.TCSETS, t)
}
