package kvtx_rpc_client

import (
	"errors"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
)

func TestRemoteErrorReconstructsInvalidSnapshot(t *testing.T) {
	err := remoteError(
		"backend page diagnostic",
		kvtx_rpc.KvtxRetryClass_KVTX_RETRY_CLASS_INVALID_SNAPSHOT,
	)
	if !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("error = %v, want ErrInvalidSnapshot", err)
	}
	if !strings.Contains(err.Error(), "backend page diagnostic") {
		t.Fatalf("error = %v, want diagnostic text", err)
	}
}

func TestRemoteErrorLeavesUnclassifiedErrorUntyped(t *testing.T) {
	err := remoteError(
		"backend page diagnostic",
		kvtx_rpc.KvtxRetryClass_KVTX_RETRY_CLASS_UNSPECIFIED,
	)
	if errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("error = %v, unexpectedly classified as ErrInvalidSnapshot", err)
	}
}

func TestRetryClassRoundTripsOnComplete(t *testing.T) {
	want := &kvtx_rpc.KvtxTransactionComplete{
		Error:      "backend page diagnostic",
		RetryClass: kvtx_rpc.KvtxRetryClass_KVTX_RETRY_CLASS_INVALID_SNAPSHOT,
	}
	data, err := want.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	got := new(kvtx_rpc.KvtxTransactionComplete)
	if err := got.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if got.GetRetryClass() != want.GetRetryClass() {
		t.Fatalf("retry class = %v, want %v", got.GetRetryClass(), want.GetRetryClass())
	}
	if got.GetError() != want.GetError() {
		t.Fatalf("diagnostic = %q, want %q", got.GetError(), want.GetError())
	}
	err = remoteError(got.GetError(), got.GetRetryClass())
	if !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("round-trip error = %v, want ErrInvalidSnapshot", err)
	}
}
