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
