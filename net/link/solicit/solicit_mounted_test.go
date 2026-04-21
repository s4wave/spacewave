package link_solicit

import (
	"testing"
)

func TestSolicitMountedStreamAccept(t *testing.T) {
	sms := NewSolicitMountedStream(nil).(*solicitMountedStream)
	// Accept the first time should succeed.
	ms, already, err := sms.AcceptMountedStream()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if already {
		t.Fatal("should not be already accepted")
	}
	if ms != nil {
		t.Fatal("expected nil MountedStream (we passed nil)")
	}

	// Second accept should return already=true.
	_, already, err = sms.AcceptMountedStream()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !already {
		t.Fatal("should be already accepted on second call")
	}
}

func TestSolicitMountedStreamError(t *testing.T) {
	sms := NewSolicitMountedStreamWithErr(errClosed)
	_, _, err := sms.AcceptMountedStream()
	if err != errClosed {
		t.Fatalf("expected errClosed, got: %v", err)
	}
}

func TestSolicitMountedStreamIsAccepted(t *testing.T) {
	sms := NewSolicitMountedStream(nil).(*solicitMountedStream)
	if sms.IsAccepted() {
		t.Fatal("should not be accepted initially")
	}
	sms.AcceptMountedStream()
	if !sms.IsAccepted() {
		t.Fatal("should be accepted after AcceptMountedStream")
	}
}

var errClosed = errSolicitationClosed
