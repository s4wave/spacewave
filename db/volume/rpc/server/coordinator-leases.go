package volume_rpc_server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/s4wave/spacewave/db/coord"
)

type coordinatorLeases struct {
	mu     sync.Mutex
	leases map[string]coord.WriteLease
}

func newCoordinatorLeases() *coordinatorLeases {
	return &coordinatorLeases{
		leases: make(map[string]coord.WriteLease),
	}
}

func (l *coordinatorLeases) add(lease coord.WriteLease) (string, error) {
	var token [16]byte
	for {
		if _, err := rand.Read(token[:]); err != nil {
			return "", err
		}
		id := hex.EncodeToString(token[:])

		l.mu.Lock()
		if _, exists := l.leases[id]; !exists {
			l.leases[id] = lease
			l.mu.Unlock()
			return id, nil
		}
		l.mu.Unlock()
	}
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
