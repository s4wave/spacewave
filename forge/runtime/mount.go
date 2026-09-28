package forge_runtime

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	"github.com/s4wave/spacewave/db/world"
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
