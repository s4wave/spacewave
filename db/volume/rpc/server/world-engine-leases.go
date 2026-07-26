package volume_rpc_server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/s4wave/spacewave/db/volume"
)

type worldEngineLeases struct {
	mu     sync.Mutex
	leases map[string]volume.WorldEngineLease
}

func newWorldEngineLeases() *worldEngineLeases {
	return &worldEngineLeases{
		leases: make(map[string]volume.WorldEngineLease),
	}
}

func (l *worldEngineLeases) add(lease volume.WorldEngineLease) string {
	// The ID doubles as the release capability shared across RPC clients,
	// so it must be unguessable rather than sequential.
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		panic(err)
	}
	id := "lease-" + hex.EncodeToString(token)

	l.mu.Lock()
	l.leases[id] = lease
	l.mu.Unlock()

	return id
}

func (l *worldEngineLeases) release(id string) error {
	l.mu.Lock()
	lease := l.leases[id]
	delete(l.leases, id)
	l.mu.Unlock()
	if lease == nil {
		return nil
	}
	return lease.Release()
}
