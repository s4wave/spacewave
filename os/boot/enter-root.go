//go:build linux

package boot

import "golang.org/x/sys/unix"

// EnterRoot replaces an independent bootstrap child with the UnixFS workload.
// It must never run in the process serving that root: exec can fault on FUSE.
func EnterRoot(root, arg string) error {
	// Enter the workload root only after bootstrap exec released the server's
	// vfork. This process has its own address space and serves no FUSE requests.
	if err := unix.Chroot(root); err != nil {
		return err
	}
	if err := unix.Chdir("/"); err != nil {
		return err
	}

	// Replace this child with the executable whose pages come from UnixFS.
	return unix.Exec("/workload", []string{"/workload", arg}, []string{"GOMAXPROCS=1"})
}

// EnterConsole replaces the independent session leader with the ordinary shell.
// PID1 already supplied a serial controlling tty as all three inherited streams.
func EnterConsole(root string) error {
	// Cross into FUSE only from the independently execed bootstrap address space.
	if err := unix.Chroot(root); err != nil {
		return err
	}
	if err := unix.Chdir("/data"); err != nil {
		return err
	}

	// Keep bootstrap paths and backing stores out of the shell's environment.
	return unix.Exec("/bin/sh", []string{"/bin/sh", "-i"}, []string{
		"PATH=/bin:/usr/bin:/usr/lib/git-core", "HOME=/data", "TMPDIR=/tmp", "TERM=vt100", "PS1=spacewave# ", "GOMAXPROCS=1",
	})
}

// Guest paths of the daemon's state directory and of the card's grant are both
// bound by RunConsole over mountpoints the image provides.
const (
	daemonPath      = "/usr/bin/spacewave"
	daemonStatePath = "/var/lib/spacewave"
	daemonGrantPath = "/etc/spacewave/grant"
)

// EnterDaemon replaces an independent bootstrap child with the image's daemon.
// With grant set, the daemon enrolls this Device through the card's grant.
func EnterDaemon(root string, grant bool) error {
	// Cross into FUSE only from the independently execed bootstrap address space.
	if err := unix.Chroot(root); err != nil {
		return err
	}
	if err := unix.Chdir(daemonStatePath); err != nil {
		return err
	}

	// The daemon outlives its clients, so idle shutdown stays off.
	args := []string{daemonPath, "serve", "--state-path", daemonStatePath, "--idle-timeout", "0"}
	if grant {
		args = append(args, "--device-grant", daemonGrantPath)
	}
	return unix.Exec(daemonPath, args, []string{
		"PATH=/bin:/usr/bin", "HOME=" + daemonStatePath, "TMPDIR=/tmp",
	})
}
