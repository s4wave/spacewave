//go:build unix

package forge_lib_docker

import (
	"os"
	"strings"
	"syscall"

	"github.com/pkg/errors"
)

// workerUser returns the Worker's own user and group, with the group of each
// mapped host device added. A container that writes outputs runs as it, so
// the files it leaves belong to the Worker that stores and removes them, and
// it reaches a device such as /dev/kvm through the device's group.
func workerUser(devices []string) (*ContainerUser, error) {
	// Start from this process's user and primary group.
	user := &ContainerUser{UID: os.Getuid(), GID: os.Getgid()}

	// Add each device node's group, once, unless it is the primary group.
	seen := map[int]bool{user.GID: true}
	for _, device := range devices {
		host, _, _ := strings.Cut(device, ":")
		info, err := os.Stat(host)
		if err != nil {
			return nil, errors.Wrap(err, "docker device")
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil, errors.Errorf("docker device %s: no owner", host)
		}
		if gid := int(st.Gid); !seen[gid] {
			seen[gid] = true
			user.Groups = append(user.Groups, gid)
		}
	}
	return user, nil
}
