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
func runGuestShell(ctx context.Context, shutdown <-chan struct{}, executable, root, terminal string) (joined bool, retErr error) {
	// Global waits and signals are forbidden in the hosted mount-namespace probe.
	if os.Getpid() != 1 {
		return false, errors.New("interactive supervision requires actual guest PID1")
	}

	// Open the serial device itself: /dev/console cannot acquire a controlling tty.
	tty, err := os.OpenFile(terminal, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return true, err
	}
	defer func() { retErr = errors.Join(retErr, tty.Close()) }()
	if err := prepareSerial(int(tty.Fd())); err != nil {
		return true, err
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
		return true, err
	}
	defer func() { retErr = errors.Join(retErr, child.Release()) }()
	defer func() {
		// Kill all guest jobs, including detached sessions, then reap to ECHILD.
		// Release closes Go's process descriptor; it never calls Cmd.Wait.
		err := containGuest()
		joined = err == nil
		retErr = errors.Join(retErr, err)
	}()
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
				return false, errors.Join(errors.New("guest child wait failed"), err)
			}
			if pid == 0 {
				break
			}
			if pid == child.Pid {
				println("SPACEWAVE_OS_SHELL_EXIT status=" + strconv.Itoa(int(status)))
				if !status.Exited() || status.ExitStatus() != 0 {
					return false, errors.New("shell exited unsuccessfully; wait status=" + strconv.Itoa(int(status)))
				}
				return false, nil
			}
		}

		// Server/controller failure cancels ctx and disconnects FUSE before reaping.
		select {
		case <-shutdown:
			println("Poweroff requested.")
			return false, nil
		case <-ctx.Done():
			return false, context.Cause(ctx)
		case <-children:
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

	// A stopped shell may leave detached jobs; process-group signals miss those.
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
