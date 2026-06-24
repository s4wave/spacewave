package volume_rpc_server

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/coord"
)

type coordinatorLeases struct {
	mu     sync.Mutex
	nextID atomic.Uint64
	leases map[string]coord.WriteLease
}

func newCoordinatorLeases() *coordinatorLeases {
	return &coordinatorLeases{
		leases: make(map[string]coord.WriteLease),
	}
}

func (l *coordinatorLeases) add(lease coord.WriteLease) string {
	id := "lease-" + strconv.FormatUint(l.nextID.Add(1), 10)

	l.mu.Lock()
	l.leases[id] = lease
	l.mu.Unlock()

	return id
}

func (l *coordinatorLeases) get(id string) (coord.WriteLease, error) {
	l.mu.Lock()
	lease := l.leases[id]
	l.mu.Unlock()
	if lease == nil {
		return nil, coord.ErrLeaseReleased
	}
	return lease, nil
}

func (l *coordinatorLeases) release(ctx context.Context, id string) error {
	l.mu.Lock()
	lease := l.leases[id]
	delete(l.leases, id)
	l.mu.Unlock()
	if lease == nil {
		return nil
	}
	return lease.Release(ctx)
}
