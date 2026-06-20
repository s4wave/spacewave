//go:build !tinygo && !goscript

package s4wave_session

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/crypto"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

type manualSignalReadyResult struct {
	rwc io.ReadWriteCloser
	err error
}

type manualSignalTestRWC struct {
	closed chan struct{}
	once   sync.Once
}

func newManualSignalTestRWC() *manualSignalTestRWC {
	return &manualSignalTestRWC{closed: make(chan struct{})}
}

func (r *manualSignalTestRWC) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (r *manualSignalTestRWC) Write(p []byte) (int, error) {
	return len(p), nil
}

func (r *manualSignalTestRWC) Close() error {
	r.once.Do(func() {
		close(r.closed)
	})
	return nil
}

func waitManualSignalReady(t *testing.T, ch <-chan manualSignalReadyResult) manualSignalReadyResult {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for manual signal state result")
		return manualSignalReadyResult{}
	}
}

func requireManualSignalClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for manual signal rwc close")
	}
}

func TestManualSignalTransportStateReadyWakesWaiter(t *testing.T) {
	var state manualSignalTransportState
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := make(chan manualSignalReadyResult, 1)
	go func() {
		rwc, err := state.waitReady(ctx)
		result <- manualSignalReadyResult{rwc: rwc, err: err}
	}()

	rwc := newManualSignalTestRWC()
	if !state.setReady(rwc) {
		t.Fatal("expected ready state to be accepted")
	}

	got := waitManualSignalReady(t, result)
	if got.err != nil {
		t.Fatalf("waitReady error: %v", got.err)
	}
	if got.rwc != rwc {
		t.Fatal("waitReady returned the wrong datachannel rwc")
	}
}

func TestManualSignalTransportStateReadyIsConsumedOnce(t *testing.T) {
	var state manualSignalTransportState
	rwc := newManualSignalTestRWC()
	if !state.setReady(rwc) {
		t.Fatal("expected ready state to be accepted")
	}

	got, err := state.waitReady(t.Context())
	if err != nil {
		t.Fatalf("waitReady error: %v", err)
	}
	if got != rwc {
		t.Fatal("waitReady returned the wrong datachannel rwc")
	}

	got, err = state.waitReady(t.Context())
	if !errors.Is(err, errManualSignalDataChannelLinked) {
		t.Fatalf("second waitReady error = %v, want %v", err, errManualSignalDataChannelLinked)
	}
	if got != nil {
		t.Fatal("second waitReady returned rwc")
	}
}

func TestManualSignalTransportStateCloseWakesWaiter(t *testing.T) {
	var state manualSignalTransportState
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := make(chan manualSignalReadyResult, 1)
	go func() {
		rwc, err := state.waitReady(ctx)
		result <- manualSignalReadyResult{rwc: rwc, err: err}
	}()

	if !state.close() {
		t.Fatal("expected close to transition state")
	}

	got := waitManualSignalReady(t, result)
	if !errors.Is(got.err, errManualSignalDataChannelClosed) {
		t.Fatalf("waitReady error = %v, want %v", got.err, errManualSignalDataChannelClosed)
	}
	if got.rwc != nil {
		t.Fatal("waitReady returned rwc after close")
	}
}

func TestManualSignalTransportStateFailWakesWaiter(t *testing.T) {
	var state manualSignalTransportState
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := make(chan manualSignalReadyResult, 1)
	go func() {
		rwc, err := state.waitReady(ctx)
		result <- manualSignalReadyResult{rwc: rwc, err: err}
	}()

	wantErr := errors.New("detach failed")
	state.fail(wantErr)

	got := waitManualSignalReady(t, result)
	if !errors.Is(got.err, wantErr) {
		t.Fatalf("waitReady error = %v, want %v", got.err, wantErr)
	}
	if got.rwc != nil {
		t.Fatal("waitReady returned rwc after failure")
	}
}

func TestManualSignalTransportStateWaitReadyContextCancellation(t *testing.T) {
	var state manualSignalTransportState
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	got, err := state.waitReady(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitReady error = %v, want %v", err, context.Canceled)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after context cancellation")
	}
}

func TestManualSignalTransportStateKeepsFailureAfterClose(t *testing.T) {
	var state manualSignalTransportState
	wantErr := errors.New("datachannel failed")

	state.fail(wantErr)
	state.close()

	got, err := state.waitReady(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("waitReady error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after failure")
	}
}

func TestManualSignalTransportStateFailureWinsAfterClose(t *testing.T) {
	var state manualSignalTransportState
	state.close()

	wantErr := errors.New("datachannel failed")
	state.fail(wantErr)

	got, err := state.waitReady(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("waitReady error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after failure")
	}
}

func TestManualSignalTransportStateFailureWinsOverReady(t *testing.T) {
	var state manualSignalTransportState
	rwc := newManualSignalTestRWC()
	if !state.setReady(rwc) {
		t.Fatal("expected ready state to be accepted")
	}

	wantErr := errors.New("ready channel failed")
	state.fail(wantErr)

	got, err := state.waitReady(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("waitReady error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after failure")
	}
	requireManualSignalClosed(t, rwc.closed)
}

func TestManualSignalTransportStateCloseCleansPendingReady(t *testing.T) {
	var state manualSignalTransportState
	rwc := newManualSignalTestRWC()
	if !state.setReady(rwc) {
		t.Fatal("expected ready state to be accepted")
	}

	if !state.close() {
		t.Fatal("expected close to transition state")
	}
	requireManualSignalClosed(t, rwc.closed)

	got, err := state.waitReady(t.Context())
	if !errors.Is(err, errManualSignalDataChannelClosed) {
		t.Fatalf("waitReady error = %v, want %v", err, errManualSignalDataChannelClosed)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after close")
	}
}

func TestManualSignalTransportDropsLateReadyAfterClose(t *testing.T) {
	m := &ManualSignalTransport{}
	if !m.state.close() {
		t.Fatal("expected close to transition state")
	}

	rwc := newManualSignalTestRWC()
	m.onDataChannelReady(rwc)

	requireManualSignalClosed(t, rwc.closed)

	got, err := m.state.waitReady(t.Context())
	if !errors.Is(err, errManualSignalDataChannelClosed) {
		t.Fatalf("waitReady error = %v, want %v", err, errManualSignalDataChannelClosed)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after close")
	}
}

func TestManualSignalTransportNilReadyFailsWithoutPanic(t *testing.T) {
	m := &ManualSignalTransport{}
	m.onDataChannelReady(nil)

	got, err := m.state.waitReady(t.Context())
	if !errors.Is(err, errManualSignalDataChannelClosed) {
		t.Fatalf("waitReady error = %v, want %v", err, errManualSignalDataChannelClosed)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after nil ready")
	}
}

func TestManualSignalTransportWaitLinkClosesReadyRWCOnQuicFailure(t *testing.T) {
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	localPeerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := p2ptls.NewIdentity(priv)
	if err != nil {
		t.Fatal(err)
	}

	remotePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerID, err := peer.IDFromPrivateKey(remotePriv)
	if err != nil {
		t.Fatal(err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	m := &ManualSignalTransport{
		identity:  identity,
		localPeer: localPeerID,
		offerer:   true,
		le:        logrus.NewEntry(logger),
	}
	rwc := newManualSignalTestRWC()
	if !m.state.setReady(rwc) {
		t.Fatal("expected ready state to be accepted")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	lnk, err := m.WaitLink(ctx, t.Context(), remotePeerID)
	if err == nil {
		_ = lnk.Close()
		t.Fatal("expected quic session error")
	}
	requireManualSignalClosed(t, rwc.closed)
}
