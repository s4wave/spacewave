//go:build linux

package boot

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// RunMountHelper implements bazil's privileged system/data mount handshake.
// This is neither a setuid helper nor a replacement for general fusermount3.
func RunMountHelper(args []string) error {
	// Accept the adapter's fixed filesystem identity and optional read-only flag.
	if os.Geteuid() != 0 || len(args) != 4 || args[0] != "-o" || args[2] != "--" {
		return errors.New("mount helper requires root and bazil mount arguments")
	}
	options := strings.Split(args[1], ",")
	if len(options) < 2 || len(options) > 3 {
		return errors.New("mount helper requires fsname, subtype and optional ro")
	}
	seen := make(map[string]bool, 3)
	for _, option := range options {
		if seen[option] || (option != "ro" && option != "fsname=hydrafs" && option != "subtype=hydrafs") {
			return errors.New("unsupported mount option: " + option)
		}
		seen[option] = true
	}
	if !seen["fsname=hydrafs"] || !seen["subtype=hydrafs"] {
		return errors.New("mount helper requires the UnixFS filesystem identity")
	}
	comm, err := strconv.Atoi(os.Getenv("_FUSE_COMMFD"))
	if err != nil || comm != 3 {
		return errors.New("mount helper requires bazil communication fd 3")
	}

	// Open the kernel channel outside the served root and mount it directly.
	fd, err := unix.Open("/dev/fuse", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	data := "fd=" + strconv.Itoa(fd) + ",rootmode=40000,user_id=0,group_id=0"
	flags := uintptr(unix.MS_NOSUID | unix.MS_NODEV)
	if seen["ro"] {
		flags |= unix.MS_RDONLY
	}
	if err := unix.Mount("spacewave-os", args[3], "fuse.hydrafs", flags, data); err != nil {
		return err
	}

	// Transfer the open channel to bazil; undo the mount if it cannot receive it.
	if err := unix.Sendmsg(comm, []byte{0}, unix.UnixRights(fd), nil, 0); err != nil {
		return errors.Join(err, unix.Unmount(args[3], 0))
	}
	return nil
}
