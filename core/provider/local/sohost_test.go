package provider_local

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
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

	_, rejected, err := host.WaitOperation(ctx, localID)
	if !rejected {
		t.Fatal("rejected local result did not return rejected")
	}
	if !errors.Is(err, sobject.ErrRejectedOp) {
		t.Fatalf("err = %v, want ErrRejectedOp", err)
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
		nonce,
		localID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return op
}
