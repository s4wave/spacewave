package rpc_kvtx_server

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"

	rpc_kvtx "github.com/aperturerobotics/bldr/rpc/kvtx"
	"github.com/aperturerobotics/hydra/kvtx"
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
	txs map[string]srpc.Mux
}

// NewStore constructs a new Store.
func NewStore(store kvtx.Store) *Store {
	return &Store{store: store}
}

// KvtxTransaction starts & manages a key-value transaction.
func (s *Store) KvtxTransaction(strm rpc_kvtx.SRPCKvtx_KvtxTransactionStream) error {
	req, err := strm.Recv()
	if err != nil {
		return err
	}

	write := req.GetInit().GetWrite()
	tx, err := s.store.NewTransaction(write)
	var errStr, txID string
	if err != nil {
		errStr = err.Error()
	} else {
		txIDNumeric := s.idCounter.Add(1) - 1
		txID = "tx/" + strconv.Itoa(int(txIDNumeric))

		mux := srpc.NewMux()
		if err := rpc_kvtx.SRPCRegisterKvtxOps(mux, NewOps(tx)); err != nil {
			tx.Discard()
			return err
		}

		s.rmtx.Lock()
		s.txs[txID] = mux
		s.rmtx.Unlock()
	}

	// ensure tx is discarded and removed on return
	defer func() {
		if tx != nil {
			tx.Discard()
		}
		if txID != "" {
			s.rmtx.Lock()
			delete(s.txs, txID)
			s.rmtx.Unlock()
		}
	}()

	txErr := strm.Send(&rpc_kvtx.KvtxTransactionResponse{
		Body: &rpc_kvtx.KvtxTransactionResponse_Ack{
			Ack: &rpc_kvtx.KvtxTransactionAck{
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
	if doCommit {
		commitErr = tx.Commit(strm.Context())
		if commitErr != nil {
			commitErrStr = commitErr.Error()
		}
	} else {
		tx.Discard()
	}

	return strm.Send(&rpc_kvtx.KvtxTransactionResponse{
		Body: &rpc_kvtx.KvtxTransactionResponse_Complete{
			Complete: &rpc_kvtx.KvtxTransactionComplete{
				Error:     commitErrStr,
				Committed: doCommit && commitErr == nil,
				Discarded: doDiscard || commitErr != nil,
			},
		},
	})
}

// KvtxTransactionRpc proxies a RPC to the KvtxOps service for the transaction.
func (s *Store) KvtxTransactionRpc(strm rpc_kvtx.SRPCKvtx_KvtxTransactionRpcStream) error {
	return rpcstream.HandleRpcStream(strm, s.GetKvtxOpsMux)
}

// GetKvtxOpsMux returns the KvtxOpsServer mux for the given transaction id.
func (s *Store) GetKvtxOpsMux(ctx context.Context, txID string) (srpc.Mux, func(), error) {
	s.rmtx.RLock()
	mux, ok := s.txs[txID]
	s.rmtx.RUnlock()
	if !ok {
		return nil, nil, kvtx.ErrDiscarded
	}
	return mux, nil, nil
}

// _ is a type assertion
var _ rpc_kvtx.SRPCKvtxServer = ((*Store)(nil))
