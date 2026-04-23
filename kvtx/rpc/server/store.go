package kvtx_rpc_server

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_rpc "github.com/aperturerobotics/hydra/kvtx/rpc"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// Store wraps a kvtx store in a RPC service.
type Store struct {
	// store is the underlying kvtx store
	store kvtx.Store
	// idCounter is the transaction id counter.
	idCounter atomic.Uint32
	// rmtx guards below fields
	rmtx sync.RWMutex
	// txs is the list of ongoing transaction ops.
	txs map[string]*txHandle
}

type txHandle struct {
	// mux is the ops service mux for the transaction.
	mux srpc.Mux

	// mtx guards below fields.
	mtx sync.Mutex
	// closing indicates commit/discard has started.
	closing bool
	// active tracks active ops streams by id.
	active map[uint64]func()
	// next is the next active stream id.
	next uint64
	// idle closes when closing is set and active is empty.
	idle chan struct{}
}

// NewStore constructs a new Store.
func NewStore(store kvtx.Store) *Store {
	return &Store{
		store: store,
		txs:   make(map[string]*txHandle),
	}
}

func newTxHandle(tx kvtx.Tx) (*txHandle, error) {
	mux := srpc.NewMux()
	if err := kvtx_rpc.SRPCRegisterKvtxOps(mux, NewOps(tx)); err != nil {
		return nil, err
	}
	return &txHandle{
		mux:    mux,
		active: make(map[uint64]func()),
		idle:   make(chan struct{}),
	}, nil
}

func (h *txHandle) acquire(released func()) (srpc.Invoker, func(), error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	if h.closing {
		return nil, nil, kvtx.ErrDiscarded
	}

	id := h.next
	h.next++
	h.active[id] = released

	return h.mux, func() {
		h.release(id)
	}, nil
}

func (h *txHandle) release(id uint64) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	delete(h.active, id)
	if h.closing && len(h.active) == 0 && h.idle != nil {
		close(h.idle)
		h.idle = nil
	}
}

func (h *txHandle) closeOps() {
	h.mtx.Lock()
	if h.closing {
		idle := h.idle
		h.mtx.Unlock()
		if idle != nil {
			<-idle
		}
		return
	}

	h.closing = true
	releases := make([]func(), 0, len(h.active))
	for _, release := range h.active {
		if release != nil {
			releases = append(releases, release)
		}
	}
	idle := h.idle
	if len(h.active) == 0 && h.idle != nil {
		close(h.idle)
		h.idle = nil
		idle = nil
	}
	h.mtx.Unlock()

	for _, release := range releases {
		release()
	}
	if idle != nil {
		<-idle
	}
}

// KvtxTransaction starts & manages a key-value transaction.
func (s *Store) KvtxTransaction(strm kvtx_rpc.SRPCKvtx_KvtxTransactionStream) error {
	req, err := strm.Recv()
	if err != nil {
		return err
	}

	write := req.GetInit().GetWrite()
	tx, err := s.store.NewTransaction(strm.Context(), write)
	var errStr, txID string
	if err != nil {
		errStr = err.Error()
	} else {
		txIDNumeric := s.idCounter.Add(1) - 1
		txID = "tx/" + strconv.Itoa(int(txIDNumeric))

		handle, hErr := newTxHandle(tx)
		if hErr != nil {
			tx.Discard()
			return hErr
		}

		s.rmtx.Lock()
		s.txs[txID] = handle
		s.rmtx.Unlock()
	}

	// ensure tx is discarded and removed on return
	defer func() {
		var handle *txHandle
		if txID != "" {
			s.rmtx.Lock()
			handle = s.txs[txID]
			delete(s.txs, txID)
			s.rmtx.Unlock()
		}
		if handle != nil {
			handle.closeOps()
		}
		if tx != nil {
			tx.Discard()
		}
	}()

	txErr := strm.Send(&kvtx_rpc.KvtxTransactionResponse{
		Body: &kvtx_rpc.KvtxTransactionResponse_Ack{
			Ack: &kvtx_rpc.KvtxTransactionAck{
				Error:         errStr,
				TransactionId: txID,
			},
		},
	})
	if err != nil || txErr != nil {
		return txErr
	}

	// wait for commit or discard
	req, err = strm.Recv()
	if err != nil {
		return err
	}
	doCommit, doDiscard := req.GetCommit(), req.GetDiscard()
	if !doCommit && !doDiscard {
		return errors.New("expected commit or discard but got neither")
	}
	var commitErrStr string
	var commitErr error
	if txID != "" {
		s.rmtx.Lock()
		handle := s.txs[txID]
		delete(s.txs, txID)
		s.rmtx.Unlock()
		if handle != nil {
			handle.closeOps()
		}
	}
	if doCommit {
		commitErr = tx.Commit(strm.Context())
		if commitErr != nil {
			commitErrStr = commitErr.Error()
		}
	} else {
		tx.Discard()
	}

	return strm.Send(&kvtx_rpc.KvtxTransactionResponse{
		Body: &kvtx_rpc.KvtxTransactionResponse_Complete{
			Complete: &kvtx_rpc.KvtxTransactionComplete{
				Error:     commitErrStr,
				Committed: doCommit && commitErr == nil,
				Discarded: doDiscard || commitErr != nil,
			},
		},
	})
}

// KvtxTransactionRpc proxies a RPC to the KvtxOps service for the transaction.
func (s *Store) KvtxTransactionRpc(strm kvtx_rpc.SRPCKvtx_KvtxTransactionRpcStream) error {
	return rpcstream.HandleRpcStream(strm, s.GetKvtxOpsMux)
}

// GetKvtxOpsMux returns the KvtxOpsServer mux for the given transaction id.
func (s *Store) GetKvtxOpsMux(ctx context.Context, txID string, released func()) (srpc.Invoker, func(), error) {
	s.rmtx.RLock()
	handle, ok := s.txs[txID]
	s.rmtx.RUnlock()
	if !ok {
		return nil, nil, kvtx.ErrDiscarded
	}
	return handle.acquire(released)
}

// _ is a type assertion
var _ kvtx_rpc.SRPCKvtxServer = ((*Store)(nil))
