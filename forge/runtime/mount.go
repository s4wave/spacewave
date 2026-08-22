package forge_runtime

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	"github.com/s4wave/spacewave/db/world"
	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
)

// WorkdirMount is the single writer-fenced live mount of one Workdir FSHandle
// into one runtime backend. One attempt owns exactly one mount: guest writes
// enter the Workdir through the Spacewave-owned FSHandle writer, so the
// FSHandle is the only write fence. Flush fences every write that traversed
// the FSHandle into durable storage; call it once before diff evidence.
type WorkdirMount interface {
	// Flush fences pending FSHandle writes into durable storage before diff evidence.
	Flush(ctx context.Context) error
	// Release revokes guest access and tears down the backend mount.
	Release(ctx context.Context) error
}

// V86WorkdirMount registers the writable v86fs mount of one Workdir FSHandle
// in the v86fs relay server serving the VM. Guest writes traverse v86fs into
// the FSHandle, so Flush fences them with an engine durability barrier and
// Release revokes guest access by removing the mount.
type V86WorkdirMount struct {
	eng       world.Engine
	server    *unixfs_v86fs.Server
	name      string
	guestPath string
	handle    *unixfs.FSHandle

	mtx      sync.Mutex
	released bool
	attached bool
}

// NewV86WorkdirMount constructs the writable v86fs adapter for one Workdir
// FSHandle. Attach registers it once; a second Attach is rejected because one
// attempt mounts its Workdir exactly once.
func NewV86WorkdirMount(
	eng world.Engine,
	server *unixfs_v86fs.Server,
	name, guestPath string,
	handle *unixfs.FSHandle,
) (*V86WorkdirMount, error) {
	switch {
	case eng == nil:
		return nil, errors.New("world engine not set")
	case server == nil:
		return nil, errors.New("v86fs server not set")
	case name == "" || guestPath == "":
		return nil, errors.New("mount name and guest path must be set")
	case handle == nil:
		return nil, errors.New("workdir fs handle not set")
	}
	return &V86WorkdirMount{eng: eng, server: server, name: name, guestPath: guestPath, handle: handle}, nil
}

// Attach registers the writable Workdir mount with the v86fs server exactly once.
func (m *V86WorkdirMount) Attach() error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.released {
		return errors.New("workdir mount released")
	}
	if m.attached {
		return errors.New("workdir mount already attached")
	}
	m.server.AddMount(m.name, m.guestPath, m.handle)
	m.attached = true
	return nil
}

// Flush implements WorkdirMount by running the engine durability barrier over
// every FSHandle write the guest made.
func (m *V86WorkdirMount) Flush(ctx context.Context) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.released {
		return errors.New("workdir mount released")
	}
	if _, err := m.eng.Sync(ctx); err != nil {
		return errors.Wrap(err, "sync workdir writes")
	}
	return nil
}

// Release implements WorkdirMount by removing the mount, revoking guest access.
func (m *V86WorkdirMount) Release(_ context.Context) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.released {
		return nil
	}
	if m.attached {
		m.server.RemoveMount(m.name)
	}
	m.released = true
	return nil
}

// ApplyDockerWorkdirBind adds the single POSIX bind-mount entry for one
// supervised Workdir host path to a Docker config.
//
// This adapter does not fence writes: a Docker bind mount lets the container
// write the host path directly, bypassing any FSHandle writer. The missing
// piece is a supervised POSIX live-FSHandle mount - a FUSE supervisor backed
// by db/unixfs/mount MountController exposing the Workdir FSHandle at the
// host path - owned by the Spacewave filesystem stack. Until that supervisor
// supplies the host path and its flush, Docker-backed attempts must collect
// diff evidence only after their own supervisor proves quiescence; this
// package provides no flush contract for them.
func ApplyDockerWorkdirBind(conf *forge_lib_docker.Config, hostPath, containerPath string) error {
	switch {
	case conf == nil:
		return errors.New("docker config not set")
	case hostPath == "":
		return errors.New("host path must be set")
	case containerPath == "":
		return errors.New("container path must be set")
	}
	conf.Mounts = append(conf.Mounts, &forge_lib_docker.Mount{
		HostPath:      hostPath,
		ContainerPath: containerPath,
	})
	return nil
}
