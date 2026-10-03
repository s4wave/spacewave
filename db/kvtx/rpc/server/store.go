package kvtx_rpc_server

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
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

func retryClassForError(err error) kvtx_rpc.KvtxRetryClass {
	if errors.Is(err, kvtx.ErrInvalidSnapshot) {
		return kvtx_rpc.KvtxRetryClass_KVTX_RETRY_CLASS_INVALID_SNAPSHOT
	}
	return kvtx_rpc.KvtxRetryClass_KVTX_RETRY_CLASS_UNSPECIFIED
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
	// Hold the transaction handle lock while acquiring an operations stream.
	h.mtx.Lock()
	defer h.mtx.Unlock()

	// Reject operations streams after the transaction handle starts closing.
	if h.closing {
		return nil, nil, kvtx.ErrDiscarded
	}

	// Register the operations stream and return its transaction route.
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
	// Join an existing transaction operations shutdown under the handle lock.
	h.mtx.Lock()
	if h.closing {
		idle := h.idle
		h.mtx.Unlock()
		if idle != nil {
			<-idle
		}
		return
	}

	// Mark the transaction handle closing and collect its active stream releases.
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

	// Cancel the transaction operations streams and wait for their releases.
	for _, release := range releases {
		release()
	}
	if idle != nil {
		<-idle
	}
}

// KvtxTransaction starts & manages a key-value transaction.
func (s *Store) KvtxTransaction(strm kvtx_rpc.SRPCKvtx_KvtxTransactionStream) error {
	// Receive the KV transaction initialization request from the RPC stream.
	req, err := strm.Recv()
	if err != nil {
		return err
	}

	// Open the KV transaction and register its operations route when successful.
	write := req.GetInit().GetWrite()
	tx, err := s.store.NewTransaction(strm.Context(), write)
	var errStr, txID string
	var retryClass kvtx_rpc.KvtxRetryClass
	if err != nil {
		errStr = err.Error()
		retryClass = retryClassForError(err)
	} else {
		// Assign an RPC transaction identifier for the opened KV transaction.
		txIDNumeric := s.idCounter.Add(1) - 1
		txID = "tx/" + strconv.Itoa(int(txIDNumeric))
		retryClass = kvtx_rpc.KvtxRetryClass_KVTX_RETRY_CLASS_UNSPECIFIED

		// Create the KV operations handle for the RPC transaction.
		handle, hErr := newTxHandle(tx)
		if hErr != nil {
			tx.Discard()
			return hErr
		}

		// Publish the KV transaction route under the store registry lock.
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

	// Acknowledge the KV transaction identifier or its storage failure.
	txErr := strm.Send(&kvtx_rpc.KvtxTransactionResponse{
		Body: &kvtx_rpc.KvtxTransactionResponse_Ack{
			Ack: &kvtx_rpc.KvtxTransactionAck{
				Error:         errStr,
				TransactionId: txID,
				RetryClass:    retryClass,
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

	// Close the KV operations route before committing or discarding the transaction.
	var commitErrStr string
	var commitErr error
	var commitRetryClass kvtx_rpc.KvtxRetryClass
	if txID != "" {
		s.rmtx.Lock()
		handle := s.txs[txID]
		delete(s.txs, txID)
		s.rmtx.Unlock()
		if handle != nil {
			handle.closeOps()
		}
	}

	// Commit or discard the KV transaction and retain its completion error.
	if doCommit {
		commitErr = tx.Commit(strm.Context())
		if commitErr != nil {
			commitErrStr = commitErr.Error()
			commitRetryClass = retryClassForError(commitErr)
		}
	} else {
		tx.Discard()
	}

	return strm.Send(&kvtx_rpc.KvtxTransactionResponse{
		Body: &kvtx_rpc.KvtxTransactionResponse_Complete{
			Complete: &kvtx_rpc.KvtxTransactionComplete{
				Error:      commitErrStr,
				Committed:  doCommit && commitErr == nil,
				Discarded:  doDiscard || commitErr != nil,
				RetryClass: commitRetryClass,
			},
		},
	})
}

// KvtxTransactionRpc proxies a RPC to the KvtxOps service for the transaction.
func (s *Store) KvtxTransactionRpc(strm kvtx_rpc.SRPCKvtx_KvtxTransactionRpcStream) error {
	return rpcstream.HandleRpcStream(strm, s.GetKvtxOpsMux)
}

// Watch streams key/value snapshots after committed store changes.
func (s *Store) Watch(req *kvtx_rpc.KvtxWatchRequest, strm kvtx_rpc.SRPCKvtx_WatchStream) error {
	// Stream a bounded KV watch when the request specifies snapshot limits.
	limits := kvtx.WatchLimits{MaxRecords: req.GetMaxRecords(), MaxBytes: req.GetMaxBytes()}
	if limits.MaxRecords != 0 || limits.MaxBytes != 0 {
		bounded, ok := s.store.(kvtx.BoundedWatchStore)
		if !ok {
			return strm.Send(&kvtx_rpc.KvtxWatchResponse{Error: kvtx.ErrWatchUnsupported.Error()})
		}
		err := bounded.WatchPrefixBounded(strm.Context(), req.GetPrefix(), limits, func(entries []kvtx.WatchEntry) error {
			return sendWatchResponse(strm, req, entries)
		})
		if errors.Is(err, kvtx.ErrWatchLimit) {
			return strm.Send(&kvtx_rpc.KvtxWatchResponse{Error: err.Error(), LimitExceeded: true})
		}
		return err
	}

	// Retain the unbounded watch path for public compatibility.
	watchStore, ok := s.store.(kvtx.WatchStore)
	if !ok {
		return strm.Send(&kvtx_rpc.KvtxWatchResponse{Error: kvtx.ErrWatchUnsupported.Error()})
	}
	return watchStore.WatchPrefix(strm.Context(), req.GetPrefix(), func(entries []kvtx.WatchEntry) error {
		return sendWatchResponse(strm, req, entries)
	})
}

// sendWatchResponse encodes one watched snapshot for the stream.
func sendWatchResponse(strm kvtx_rpc.SRPCKvtx_WatchStream, req *kvtx_rpc.KvtxWatchRequest, entries []kvtx.WatchEntry) error {
	resp := &kvtx_rpc.KvtxWatchResponse{
		Entries: make([]*kvtx_rpc.KvtxWatchEntry, 0, len(entries)),
	}
	for _, entry := range entries {
		watchEntry := &kvtx_rpc.KvtxWatchEntry{Key: entry.Key}
		if !req.GetOnlyKeys() {
			watchEntry.Value = entry.Value
		}
		resp.Entries = append(resp.Entries, watchEntry)
	}
	return strm.Send(resp)
}

// GetKvtxOpsMux returns the KvtxOpsServer mux for the given transaction id.
func (s *Store) GetKvtxOpsMux(ctx context.Context, txID string, released func()) (srpc.Invoker, func(), error) {
	// Resolve the KV transaction handle from the store route registry.
	s.rmtx.RLock()
	handle, ok := s.txs[txID]
	s.rmtx.RUnlock()
	if !ok {
		return nil, nil, kvtx.ErrDiscarded
	}
	return handle.acquire(released)
}

// _ is a type assertion
var _ kvtx_rpc.SRPCKvtxServer = (*Store)(nil)
