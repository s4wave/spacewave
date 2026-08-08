package sql_rpc_server

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	"github.com/s4wave/spacewave/db/tx"
)

// Store wraps a SQL store in a RPC service.
type Store struct {
	// store is the underlying SQL store.
	store hydra_sql.SqlStore
	// idCounter is the transaction id counter.
	idCounter atomic.Uint32
	// rmtx guards txs.
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
func NewStore(store hydra_sql.SqlStore) *Store {
	return &Store{
		store: store,
		txs:   make(map[string]*txHandle),
	}
}

func newTxHandle(ctx context.Context, sqlTx hydra_sql.SqlTransaction) (*txHandle, error) {
	ops, err := sqlTx.GetSqlOps(ctx)
	if err != nil {
		return nil, err
	}
	mux := srpc.NewMux()
	if err := sql_rpc.SRPCRegisterSqlOps(mux, NewOps(ops)); err != nil {
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
		return nil, nil, tx.ErrDiscarded
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

// SqlTransaction starts and manages a SQL transaction.
func (s *Store) SqlTransaction(strm sql_rpc.SRPCSql_SqlTransactionStream) error {
	req, err := strm.Recv()
	if err != nil {
		return err
	}
	init := req.GetInit()
	if init == nil {
		return errors.New("expected init request")
	}

	sqlTx, err := s.store.NewSqlTransaction(strm.Context(), init.GetWrite(), init.GetDsn())
	var errStr, txID string
	if err != nil {
		errStr = err.Error()
	} else {
		txIDNumeric := s.idCounter.Add(1) - 1
		txID = "tx/" + strconv.Itoa(int(txIDNumeric))

		handle, hErr := newTxHandle(strm.Context(), sqlTx)
		if hErr != nil {
			sqlTx.Discard()
			return hErr
		}

		s.rmtx.Lock()
		s.txs[txID] = handle
		s.rmtx.Unlock()
	}

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
		if sqlTx != nil {
			sqlTx.Discard()
		}
	}()

	txErr := strm.Send(&sql_rpc.SqlTransactionResponse{
		Body: &sql_rpc.SqlTransactionResponse_Ack{
			Ack: &sql_rpc.SqlTransactionAck{
				Error:         errStr,
				TransactionId: txID,
			},
		},
	})
	if err != nil || txErr != nil {
		return txErr
	}

	req, err = strm.Recv()
	if err != nil {
		return err
	}
	doCommit, doDiscard := req.GetCommit(), req.GetDiscard()
	if !doCommit && !doDiscard {
		return errors.New("expected commit or discard but got neither")
	}

	var completeErrStr string
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
		commitErr = sqlTx.Commit(strm.Context())
		if commitErr != nil {
			completeErrStr = commitErr.Error()
		}
	} else {
		sqlTx.Discard()
	}

	return strm.Send(&sql_rpc.SqlTransactionResponse{
		Body: &sql_rpc.SqlTransactionResponse_Complete{
			Complete: &sql_rpc.SqlTransactionComplete{
				Error:     completeErrStr,
				Committed: doCommit && commitErr == nil,
				Discarded: doDiscard || commitErr != nil,
			},
		},
	})
}

// SqlTransactionRpc proxies a RPC to the SqlOps service for the transaction.
func (s *Store) SqlTransactionRpc(strm sql_rpc.SRPCSql_SqlTransactionRpcStream) error {
	return rpcstream.HandleRpcStream(strm, s.GetSqlOpsMux)
}

// GetSqlOpsMux returns the SqlOpsServer mux for the given transaction id.
func (s *Store) GetSqlOpsMux(ctx context.Context, txID string, released func()) (srpc.Invoker, func(), error) {
	s.rmtx.RLock()
	handle, ok := s.txs[txID]
	s.rmtx.RUnlock()
	if !ok {
		return nil, nil, tx.ErrDiscarded
	}
	return handle.acquire(released)
}

// _ is a type assertion.
var _ sql_rpc.SRPCSqlServer = (*Store)(nil)
