package provider_local

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/db/object"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

const testSharedObjectID = "test-shared-object"

func TestWriteAcceptedLocalOpResultsPersistsSuccess(t *testing.T) {
	ctx := context.Background()
	host, localPeer := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	op := buildTestOperation(t, host, localID, 1)

	err := host.writeAcceptedLocalOpResults(ctx, &sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 1},
		Ops:  []*sobject.SOOperation{op},
	}, &sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 2},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := host.readLocalOpResult(ctx, localID)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("accepted result was not written")
	}
	if got := result.GetRootSeqno(); got != 2 {
		t.Fatalf("root seqno = %d, want 2", got)
	}
	if !result.GetResult().GetSuccess() {
		t.Fatal("accepted result is not marked successful")
	}
	if got := result.GetResult().GetOpRef().GetPeerId(); got != localPeer.GetPeerID().String() {
		t.Fatalf("result peer = %q, want %q", got, localPeer.GetPeerID().String())
	}
	if got := result.GetResult().GetOpRef().GetNonce(); got != 1 {
		t.Fatalf("result nonce = %d, want 1", got)
	}
}

func TestDecodeLocalRejectionUsesValidatorSigner(t *testing.T) {
	host, localPeer := newTestLocalSOHost(t)
	validator, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	validatorKey, err := validator.GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rejection, err := sobject.BuildSOOperationRejection(
		validatorKey,
		testSharedObjectID,
		localPeer.GetPeerID(),
		1,
		sobject.NewSOOperationLocalID(),
		&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "rejected"},
	)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := rejection.UnmarshalInner()
	if err != nil {
		t.Fatal(err)
	}
	details, err := host.decodeLocalRejectionError(rejection, inner)
	if err != nil {
		t.Fatal(err)
	}
	if details.GetErrorMsg() != "rejected" {
		t.Fatalf("error details = %q, want rejected", details.GetErrorMsg())
	}
}

func TestWaitOperationDecodesHostRejection(t *testing.T) {
	// Create the submitting host and a separate validator.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	host, localPeer := newTestLocalSOHost(t)
	validator, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	validatorKey, err := validator.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Reject the operation from the validator in the host state.
	localID := sobject.NewSOOperationLocalID()
	rejection, err := sobject.BuildSOOperationRejection(
		validatorKey,
		testSharedObjectID,
		localPeer.GetPeerID(),
		1,
		localID,
		&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "object already exists"},
	)
	if err != nil {
		t.Fatal(err)
	}
	stateCtr := ccontainer.NewCContainer(&sobject.SOState{
		OpRejections: []*sobject.SOPeerOpRejections{{
			PeerId:     localPeer.GetPeerID().String(),
			Rejections: []*sobject.SOOperationRejection{rejection},
		}},
	})
	host.soHost = sobject.NewSOHost(
		ctx,
		func(context.Context, string, func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
			return stateCtr, nil, nil
		},
		nil,
		testSharedObjectID,
	)

	// The submitter decrypts the details under the validator's identity.
	_, rejected, err := host.WaitOperation(ctx, localID)
	if !rejected || !errors.Is(err, sobject.ErrRejectedOp) {
		t.Fatalf("rejected = %v, err = %v, want a rejection", rejected, err)
	}
	if !strings.Contains(err.Error(), "object already exists") {
		t.Fatalf("err = %v, want the validator's message", err)
	}
}

// queueWatch signals each wait for a change from a held host state.
type queueWatch struct {
	*ccontainer.CContainer[*sobject.SOState]
	// waiting receives a signal when a waiter holds a state.
	waiting chan struct{}
}

// WaitValueChange signals the waiter before waiting for a change from old.
func (w *queueWatch) WaitValueChange(ctx context.Context, old *sobject.SOState, errCh <-chan error) (*sobject.SOState, error) {
	if old != nil {
		select {
		case w.waiting <- struct{}{}:
		default:
		}
	}
	return w.CContainer.WaitValueChange(ctx, old, errCh)
}

// TestWaitOperationReportsDroppedOp fails an operation that leaves the host
// queue with no result, no rejection and no accepted nonce.
func TestWaitOperationReportsDroppedOp(t *testing.T) {
	// Queue the operation in the host state.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	host, _ := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	op := buildTestOperation(t, host, localID, 2)
	watch := &queueWatch{
		CContainer: ccontainer.NewCContainer(&sobject.SOState{Ops: []*sobject.SOOperation{op}}),
		waiting:    make(chan struct{}, 1),
	}
	host.soHost = sobject.NewSOHost(
		ctx,
		func(context.Context, string, func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
			return watch, nil, nil
		},
		nil,
		testSharedObjectID,
	)

	// Drop the operation once the waiter has seen it queued.
	errCh := make(chan error, 1)
	go func() {
		_, _, err := host.WaitOperation(ctx, localID)
		errCh <- err
	}()
	select {
	case <-watch.waiting:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	watch.SetValue(&sobject.SOState{Root: &sobject.SORoot{InnerSeqno: 2}})
	if err := <-errCh; !errors.Is(err, sobject.ErrDroppedOp) {
		t.Fatalf("err = %v, want ErrDroppedOp", err)
	}
}

func TestWriteAcceptedLocalOpResultsSkipsPendingOps(t *testing.T) {
	ctx := context.Background()
	host, _ := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	op := buildTestOperation(t, host, localID, 1)

	err := host.writeAcceptedLocalOpResults(ctx, &sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 1},
		Ops:  []*sobject.SOOperation{op},
	}, &sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 2},
		Ops:  []*sobject.SOOperation{op},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := host.readLocalOpResult(ctx, localID)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatalf("unexpected accepted result for pending op: %#v", result)
	}
}

func TestWriteAcceptedLocalOpResultsSkipsRejectedOps(t *testing.T) {
	ctx := context.Background()
	host, localPeer := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	op := buildTestOperation(t, host, localID, 1)
	rejection, err := sobject.BuildSOOperationRejection(
		host.privKey,
		testSharedObjectID,
		localPeer.GetPeerID(),
		1,
		localID,
		&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "rejected"},
	)
	if err != nil {
		t.Fatal(err)
	}

	err = host.writeAcceptedLocalOpResults(ctx, &sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 1},
		Ops:  []*sobject.SOOperation{op},
	}, &sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 2},
		OpRejections: []*sobject.SOPeerOpRejections{{
			PeerId:     localPeer.GetPeerID().String(),
			Rejections: []*sobject.SOOperationRejection{rejection},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := host.readLocalOpResult(ctx, localID)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatalf("unexpected accepted result for rejected op: %#v", result)
	}
}

func TestWriteAcceptedLocalOpResultsPreservesExistingResult(t *testing.T) {
	ctx := context.Background()
	host, localPeer := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	op := buildTestOperation(t, host, localID, 1)
	if err := host.writeLocalOpResult(ctx, &LocalSOOperationResult{
		LocalId:   localID,
		RootSeqno: 9,
		Result: sobject.BuildSOOperationResult(
			localPeer.GetPeerID().String(),
			1,
			true,
			nil,
		),
	}); err != nil {
		t.Fatal(err)
	}

	err := host.writeAcceptedLocalOpResults(ctx, &sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 1},
		Ops:  []*sobject.SOOperation{op},
	}, &sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 2},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := host.readLocalOpResult(ctx, localID)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.GetRootSeqno(); got != 9 {
		t.Fatalf("root seqno = %d, want existing 9", got)
	}
}

func TestWaitOperationWaitsForDurableLocalQueueTransmission(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	host, localPeer := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	pending := &sobject.QueuedSOOperation{LocalId: localID, OpData: []byte("operation")}
	handle := sobject.NewSOStateParticipantHandle(
		host.le, nil, testSharedObjectID, &sobject.SOState{}, host.privKey, host.peerID,
	)
	host.stateSnapCtr = ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](
		newLsoStateSnapshot(handle, &LocalSOState{OpQueue: []*sobject.QueuedSOOperation{pending}}),
	)

	done := make(chan struct{})
	var seqno uint64
	var rejected bool
	var waitErr error
	go func() {
		seqno, rejected, waitErr = host.WaitOperation(ctx, localID)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("WaitOperation returned while the operation remained in the durable local queue")
	case <-time.After(20 * time.Millisecond):
	}
	if err := host.writeLocalOpResult(ctx, &LocalSOOperationResult{
		LocalId:   localID,
		RootSeqno: 2,
		Result: sobject.BuildSOOperationResult(
			localPeer.GetPeerID().String(), 1, true, nil,
		),
	}); err != nil {
		t.Fatal(err)
	}
	host.stateSnapCtr.SetValue(newLsoStateSnapshot(handle, &LocalSOState{}))
	<-done
	if waitErr != nil {
		t.Fatal(waitErr)
	}
	if rejected {
		t.Fatal("accepted operation returned rejected")
	}
	if seqno != 2 {
		t.Fatalf("seqno = %d, want 2", seqno)
	}
}

func TestWaitPublishedConfigFencesBodySnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	host, _ := newTestLocalSOHost(t)
	host.publishedConfigCtr = ccontainer.NewCContainer[*sobject.SharedObjectConfig](nil)
	target := &sobject.SharedObjectConfig{ConfigChainSeqno: 7, ConfigChainHash: []byte("accepted")}

	done := make(chan error, 1)
	go func() { done <- host.waitPublishedConfig(ctx, target) }()
	host.publishedConfigCtr.SetValue(&sobject.SharedObjectConfig{ConfigChainSeqno: 6, ConfigChainHash: []byte("stale")})
	select {
	case err := <-done:
		t.Fatalf("wait returned before the body snapshot reached admission: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	host.publishedConfigCtr.SetValue(target.CloneVT())
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWaitOperationUsesAcceptedLocalResult(t *testing.T) {
	ctx := context.Background()
	host, localPeer := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	if err := host.writeLocalOpResult(ctx, &LocalSOOperationResult{
		LocalId:   localID,
		RootSeqno: 7,
		Result: sobject.BuildSOOperationResult(
			localPeer.GetPeerID().String(),
			3,
			true,
			nil,
		),
	}); err != nil {
		t.Fatal(err)
	}

	seqno, rejected, err := host.WaitOperation(ctx, localID)
	if err != nil {
		t.Fatal(err)
	}
	if rejected {
		t.Fatal("accepted local result returned rejected")
	}
	if seqno != 7 {
		t.Fatalf("seqno = %d, want 7", seqno)
	}
}

func TestWaitOperationUsesPersistedAcceptedLocalResultAfterRestart(t *testing.T) {
	ctx := context.Background()
	host, localPeer := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	if err := host.writeLocalOpResult(ctx, &LocalSOOperationResult{
		LocalId:   localID,
		RootSeqno: 7,
		Result: sobject.BuildSOOperationResult(
			localPeer.GetPeerID().String(),
			3,
			true,
			nil,
		),
	}); err != nil {
		t.Fatal(err)
	}
	restartedHost := &LocalSOHost{
		le:             host.le,
		privKey:        host.privKey,
		peerID:         host.peerID,
		pubKey:         host.pubKey,
		objStore:       host.objStore,
		sharedObjectID: host.sharedObjectID,
		soHost:         sobject.NewSOHost(nil, nil, nil, testSharedObjectID),
	}

	seqno, rejected, err := restartedHost.WaitOperation(ctx, localID)
	if err != nil {
		t.Fatal(err)
	}
	if rejected {
		t.Fatal("accepted local result returned rejected")
	}
	if seqno != 7 {
		t.Fatalf("seqno = %d, want 7", seqno)
	}
}

func TestWaitOperationUsesRejectedLocalResult(t *testing.T) {
	// Persist a plain rejection.
	ctx := context.Background()
	host, localPeer := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	if err := host.writeLocalOpResult(ctx, &LocalSOOperationResult{
		LocalId:   localID,
		RootSeqno: 7,
		Result: sobject.BuildSOOperationResult(
			localPeer.GetPeerID().String(),
			3,
			false,
			&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "rejected"},
		),
	}); err != nil {
		t.Fatal(err)
	}

	// The rejection matches only ErrRejectedOp.
	_, rejected, err := host.WaitOperation(ctx, localID)
	if !rejected {
		t.Fatal("rejected local result did not return rejected")
	}
	if !errors.Is(err, sobject.ErrRejectedOp) || errors.Is(err, block.ErrNotFound) {
		t.Fatalf("err = %v, want only ErrRejectedOp", err)
	}
}

func TestWaitOperationReportsMissingBlockRejection(t *testing.T) {
	// Persist a rejection for a block the validator could not read.
	ctx := context.Background()
	host, localPeer := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	if err := host.writeLocalOpResult(ctx, &LocalSOOperationResult{
		LocalId: localID,
		Result: sobject.BuildSOOperationResult(
			localPeer.GetPeerID().String(),
			3,
			false,
			&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "world block is missing", MissingBlock: true},
		),
	}); err != nil {
		t.Fatal(err)
	}

	// The rejection also matches block.ErrNotFound.
	_, rejected, err := host.WaitOperation(ctx, localID)
	if !rejected || !errors.Is(err, sobject.ErrRejectedOp) || !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("rejected = %v, err = %v, want a missing-block rejection", rejected, err)
	}
}

func TestLocalOpResultOutcomeLeavesLegacySuccessUnresolved(t *testing.T) {
	ctx := context.Background()
	host, localPeer := newTestLocalSOHost(t)
	localID := sobject.NewSOOperationLocalID()
	if err := host.writeLocalOpResult(ctx, &LocalSOOperationResult{
		LocalId: localID,
		Result: sobject.BuildSOOperationResult(
			localPeer.GetPeerID().String(),
			3,
			true,
			nil,
		),
	}); err != nil {
		t.Fatal(err)
	}

	seqno, rejected, err, resolved := host.localOpResultOutcome(ctx, localID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved {
		t.Fatalf("legacy success resolved with seqno=%d rejected=%v", seqno, rejected)
	}
}

func TestLocalSOStateWriteReplaysIdentically(t *testing.T) {
	run := func(t *testing.T, injectFault bool) ([]byte, *kvtest.FaultStore) {
		t.Helper()
		ctx := t.Context()
		backend := store_kvtx_inmem.NewStore()
		var store object.ObjectStore = backend
		var faultStore *kvtest.FaultStore
		if injectFault {
			faultStore = kvtest.NewFaultStore(backend, kvtest.FaultBeforeCommit)
			store = faultStore
		}
		host := &LocalSOHost{
			objStore: store,
			soHost:   sobject.NewSOHost(nil, nil, nil, testSharedObjectID),
		}
		if err := host.writeLocalState(ctx, &LocalSOState{
			OpQueue: []*sobject.QueuedSOOperation{{
				LocalId: "replay-local-op",
				OpData:  []byte("operation-data"),
			}},
		}); err != nil {
			t.Fatal(err)
		}

		host.objStore = backend
		state, err := host.readLocalState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		data, err := state.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		return data, faultStore
	}

	want, _ := run(t, false)
	got, faultStore := run(t, true)
	if !bytes.Equal(got, want) {
		t.Fatalf("local state = %x, want %x", got, want)
	}
	if got := faultStore.Opened(); got != 2 {
		t.Fatalf("opened transactions = %d, want 2", got)
	}
	if got := faultStore.DelegatedCommits(); got != 1 {
		t.Fatalf("delegated commits = %d, want 1", got)
	}
}

func newTestLocalSOHost(t *testing.T) (*LocalSOHost, peer.Peer) {
	t.Helper()
	localPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	privKey, err := localPeer.GetPrivKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pubKey, err := crypto.MarshalPublicKey(privKey.GetPublic())
	if err != nil {
		t.Fatal(err)
	}
	return &LocalSOHost{
		le:             logrus.NewEntry(logrus.New()),
		privKey:        privKey,
		peerID:         localPeer.GetPeerID(),
		pubKey:         pubKey,
		objStore:       store_kvtx_inmem.NewStore(),
		sharedObjectID: testSharedObjectID,
		soHost:         sobject.NewSOHost(nil, nil, nil, testSharedObjectID),
	}, localPeer
}

func buildTestOperation(
	t *testing.T,
	host *LocalSOHost,
	localID string,
	nonce uint64,
) *sobject.SOOperation {
	t.Helper()
	op, err := sobject.BuildSOOperation(
		testSharedObjectID,
		host.privKey,
		[]byte("encoded op"),
		testOperationLink(nonce),
		localID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return op
}

// testOperationLink places an operation at nonce under fixed chain hashes.
func testOperationLink(nonce uint64) *sobject.SOOperationLink {
	link := &sobject.SOOperationLink{Nonce: nonce, ConfigHash: bytes.Repeat([]byte{0xc0}, 32)}
	if nonce > 1 {
		link.PrevOpHash = bytes.Repeat([]byte{0xa0}, 32)
	}
	return link
}
