package hashmap

import (
	"sync"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_txcache "github.com/s4wave/spacewave/db/kvtx/txcache"
)

// NewHashmapKvtxTx constructs a new hashmap kvtx transaction.
func NewHashmapKvtxTx(m *HashmapKvtx, write bool) (kvtx.Tx, error) {
	m.rmtx.RLock()
	var readCloseOnce sync.Once
	readOps := &kvtxTxOps{
		m: m,
		commitDiscardFn: func(commit bool) error {
			readCloseOnce.Do(func() {
				m.rmtx.RUnlock()
			})
			return nil
		},
	}

	tc, err := kvtx_txcache.NewTxWithCbs(
		readOps,
		write,
		func() {
			_ = readOps.commitDiscardFn(false)
		},
		func() (kvtx.Tx, error) {
			m.rmtx.Lock()
			var writeCloseOnce sync.Once
			writeOps := &kvtxTxOps{
				m: m,
				commitDiscardFn: func(commit bool) error {
					writeCloseOnce.Do(func() {
						m.rmtx.Unlock()
					})
					return nil
				},
			}
			return writeOps, nil
		},
		true,
	)
	if err != nil {
		if readOps.commitDiscardFn != nil {
			_ = readOps.commitDiscardFn(false)
		}
	}
	return tc, err
}
