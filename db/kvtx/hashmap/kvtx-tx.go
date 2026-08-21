package hashmap

import (
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_txcache "github.com/s4wave/spacewave/db/kvtx/txcache"
)

// NewHashmapKvtxTx constructs a new hashmap kvtx transaction.
func NewHashmapKvtxTx(m *HashmapKvtx, write bool) (kvtx.Tx, error) {
	m.rmtx.RLock()
	var readCloseOnce atomic.Bool
	readOps := &kvtxTxOps{
		m: m,
		commitDiscardFn: func(commit bool) error {
			if readCloseOnce.CompareAndSwap(false, true) {
				m.rmtx.RUnlock()
			}
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
			var writeCloseOnce atomic.Bool
			writeOps := &kvtxTxOps{
				m: m,
				commitDiscardFn: func(commit bool) error {
					if writeCloseOnce.CompareAndSwap(false, true) {
						m.rmtx.Unlock()
					}
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
