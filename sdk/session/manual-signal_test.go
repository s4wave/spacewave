//go:build !tinygo

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
	// Prepare a cancelable datachannel state for the readiness waiter.
	var state manualSignalTransportState
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start a waiter for the datachannel readiness transition.
	result := make(chan manualSignalReadyResult, 1)
	go func() {
		rwc, err := state.waitReady(ctx)
		result <- manualSignalReadyResult{rwc: rwc, err: err}
	}()

	// Publish the ready datachannel to wake its waiter.
	rwc := newManualSignalTestRWC()
	if !state.setReady(rwc) {
		t.Fatal("expected ready state to be accepted")
	}

	// Verify the waiter receives the published datachannel without error.
	got := waitManualSignalReady(t, result)
	if got.err != nil {
		t.Fatalf("waitReady error: %v", got.err)
	}
	if got.rwc != rwc {
		t.Fatal("waitReady returned the wrong datachannel rwc")
	}
}

func TestManualSignalTransportStateReadyIsConsumedOnce(t *testing.T) {
	// Publish a datachannel for a single readiness claim.
	var state manualSignalTransportState
	rwc := newManualSignalTestRWC()
	if !state.setReady(rwc) {
		t.Fatal("expected ready state to be accepted")
	}

	// Verify the first readiness claim returns the published datachannel.
	got, err := state.waitReady(t.Context())
	if err != nil {
		t.Fatalf("waitReady error: %v", err)
	}
	if got != rwc {
		t.Fatal("waitReady returned the wrong datachannel rwc")
	}

	// Verify the second readiness claim reports an already linked channel.
	got, err = state.waitReady(t.Context())
	if !errors.Is(err, errManualSignalDataChannelLinked) {
		t.Fatalf("second waitReady error = %v, want %v", err, errManualSignalDataChannelLinked)
	}
	if got != nil {
		t.Fatal("second waitReady returned rwc")
	}
}

func TestManualSignalTransportStateCloseWakesWaiter(t *testing.T) {
	// Prepare a cancelable datachannel state for the closure waiter.
	var state manualSignalTransportState
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start a waiter for the datachannel closure transition.
	result := make(chan manualSignalReadyResult, 1)
	go func() {
		rwc, err := state.waitReady(ctx)
		result <- manualSignalReadyResult{rwc: rwc, err: err}
	}()

	// Close the datachannel state to wake its waiter.
	if !state.close() {
		t.Fatal("expected close to transition state")
	}

	// Verify the waiter observes closure without receiving a datachannel.
	got := waitManualSignalReady(t, result)
	if !errors.Is(got.err, errManualSignalDataChannelClosed) {
		t.Fatalf("waitReady error = %v, want %v", got.err, errManualSignalDataChannelClosed)
	}
	if got.rwc != nil {
		t.Fatal("waitReady returned rwc after close")
	}
}

func TestManualSignalTransportStateFailWakesWaiter(t *testing.T) {
	// Prepare a cancelable datachannel state for the failure waiter.
	var state manualSignalTransportState
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start a waiter for the datachannel failure transition.
	result := make(chan manualSignalReadyResult, 1)
	go func() {
		rwc, err := state.waitReady(ctx)
		result <- manualSignalReadyResult{rwc: rwc, err: err}
	}()

	// Publish a detach failure to the waiting datachannel state.
	wantErr := errors.New("detach failed")
	state.fail(wantErr)

	// Verify the waiter receives the detach failure without a datachannel.
	got := waitManualSignalReady(t, result)
	if !errors.Is(got.err, wantErr) {
		t.Fatalf("waitReady error = %v, want %v", got.err, wantErr)
	}
	if got.rwc != nil {
		t.Fatal("waitReady returned rwc after failure")
	}
}

func TestManualSignalTransportStateWaitReadyContextCancellation(t *testing.T) {
	// Cancel the datachannel waiter's context before readiness.
	var state manualSignalTransportState
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Verify the canceled waiter returns cancellation without a datachannel.
	got, err := state.waitReady(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitReady error = %v, want %v", err, context.Canceled)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after context cancellation")
	}
}

func TestManualSignalTransportStateKeepsFailureAfterClose(t *testing.T) {
	// Prepare a datachannel failure to preserve through closure.
	var state manualSignalTransportState
	wantErr := errors.New("datachannel failed")

	// Fail the datachannel state before closing it.
	state.fail(wantErr)
	state.close()

	// Verify closure preserves the original datachannel failure.
	got, err := state.waitReady(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("waitReady error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after failure")
	}
}

func TestManualSignalTransportStateFailureWinsAfterClose(t *testing.T) {
	// Close the datachannel state before recording a failure.
	var state manualSignalTransportState
	state.close()

	// Publish a datachannel failure after closure.
	wantErr := errors.New("datachannel failed")
	state.fail(wantErr)

	// Verify the later failure takes precedence over datachannel closure.
	got, err := state.waitReady(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("waitReady error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after failure")
	}
}

func TestManualSignalTransportStateFailureWinsOverReady(t *testing.T) {
	// Publish a ready datachannel before recording a failure.
	var state manualSignalTransportState
	rwc := newManualSignalTestRWC()
	if !state.setReady(rwc) {
		t.Fatal("expected ready state to be accepted")
	}

	// Fail the datachannel state while its channel is pending.
	wantErr := errors.New("ready channel failed")
	state.fail(wantErr)

	// Verify the failure releases the pending channel and prevents linking.
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
	// Publish a ready datachannel before closing the state.
	var state manualSignalTransportState
	rwc := newManualSignalTestRWC()
	if !state.setReady(rwc) {
		t.Fatal("expected ready state to be accepted")
	}

	// Close the state and verify it releases the pending channel.
	if !state.close() {
		t.Fatal("expected close to transition state")
	}
	requireManualSignalClosed(t, rwc.closed)

	// Verify the closed state never returns its released channel.
	got, err := state.waitReady(t.Context())
	if !errors.Is(err, errManualSignalDataChannelClosed) {
		t.Fatalf("waitReady error = %v, want %v", err, errManualSignalDataChannelClosed)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after close")
	}
}

func TestManualSignalTransportDropsLateReadyAfterClose(t *testing.T) {
	// Close the manual transport before datachannel readiness.
	m := &ManualSignalTransport{}
	if !m.state.close() {
		t.Fatal("expected close to transition state")
	}

	// Deliver a late ready channel to the closed manual transport.
	rwc := newManualSignalTestRWC()
	m.onDataChannelReady(rwc)

	// Verify the closed transport releases the late channel.
	requireManualSignalClosed(t, rwc.closed)

	// Verify late readiness does not reopen the closed transport.
	got, err := m.state.waitReady(t.Context())
	if !errors.Is(err, errManualSignalDataChannelClosed) {
		t.Fatalf("waitReady error = %v, want %v", err, errManualSignalDataChannelClosed)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after close")
	}
}

func TestManualSignalTransportNilReadyFailsWithoutPanic(t *testing.T) {
	// Deliver a nil ready channel to the manual transport.
	m := &ManualSignalTransport{}
	m.onDataChannelReady(nil)

	// Verify nil readiness records a closed-channel failure.
	got, err := m.state.waitReady(t.Context())
	if !errors.Is(err, errManualSignalDataChannelClosed) {
		t.Fatalf("waitReady error = %v, want %v", err, errManualSignalDataChannelClosed)
	}
	if got != nil {
		t.Fatal("waitReady returned rwc after nil ready")
	}
}

func TestManualSignalTransportWaitLinkClosesReadyRWCOnQuicFailure(t *testing.T) {
	// Build the local peer identity for the QUIC failure test.
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

	// Build a distinct remote peer identity for the link attempt.
	remotePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerID, err := peer.IDFromPrivateKey(remotePriv)
	if err != nil {
		t.Fatal(err)
	}

	// Prepare a manual transport with a ready fixture datachannel.
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

	// Verify a failed QUIC link attempt closes the claimed datachannel.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	lnk, err := m.WaitLink(ctx, t.Context(), remotePeerID)
	if err == nil {
		_ = lnk.Close()
		t.Fatal("expected quic session error")
	}
	requireManualSignalClosed(t, rwc.closed)
}
