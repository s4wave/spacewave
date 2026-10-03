package yieldpolicy

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPolicyAllow asserts that resolving a prompt with allow=true
// releases the policy with a nil error.
func TestPolicyAllow(t *testing.T) {
	// Create the broker policy for an allowed socket takeover.
	b := NewBrokerWithTimeout(2 * time.Second)
	policy := b.MakePolicy("spacewave serve", "/tmp/sock")

	// Run the takeover policy while the desktop prompt awaits a decision.
	var (
		err  error
		done = make(chan struct{})
	)
	go func() {
		err = policy(context.Background())
		close(done)
	}()

	// Allow the pending desktop takeover prompt.
	prompt := waitForPrompt(t, b, time.Second)
	if err := b.ResolvePrompt(prompt.ID, true); err != nil {
		t.Fatalf("resolve prompt: %v", err)
	}

	// Require the allowed takeover policy to complete successfully.
	<-done
	if err != nil {
		t.Fatalf("policy returned error: %v", err)
	}

	// Verify that the resolved takeover prompt leaves the pending snapshot.
	pending, _ := b.SnapshotPrompts()
	if len(pending) != 0 {
		t.Fatalf("pending prompts after resolve: %d", len(pending))
	}
}

// TestPolicyDeny asserts that resolving a prompt with allow=false
// produces a deny error that names the Spacewave desktop app.
func TestPolicyDeny(t *testing.T) {
	// Create the broker policy for a denied socket takeover.
	b := NewBrokerWithTimeout(2 * time.Second)
	policy := b.MakePolicy("spacewave serve", "/tmp/sock")

	// Run the takeover policy while the desktop prompt awaits a decision.
	var (
		err  error
		done = make(chan struct{})
	)
	go func() {
		err = policy(context.Background())
		close(done)
	}()

	// Deny the pending desktop takeover prompt.
	prompt := waitForPrompt(t, b, time.Second)
	if err := b.ResolvePrompt(prompt.ID, false); err != nil {
		t.Fatalf("resolve prompt: %v", err)
	}

	// Require the denied takeover policy to identify the desktop app in its error.
	<-done
	if err == nil {
		t.Fatalf("policy returned nil error on deny")
	}
	if !strings.Contains(err.Error(), "Spacewave desktop app") {
		t.Fatalf("deny error missing app name: %v", err)
	}
}

// TestPolicyTimeout asserts that a prompt which is not resolved in
// time auto-denies with a timeout error.
func TestPolicyTimeout(t *testing.T) {
	// Create a takeover policy with a short prompt expiry.
	b := NewBrokerWithTimeout(50 * time.Millisecond)
	policy := b.MakePolicy("spacewave serve", "/tmp/sock")

	// Run the takeover policy without resolving its desktop prompt.
	start := time.Now()
	err := policy(context.Background())

	// Verify that the takeover policy waits for expiry and reports a timeout.
	if err == nil {
		t.Fatalf("policy returned nil on timeout")
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatalf("policy returned too quickly: %s", time.Since(start))
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout error missing timeout string: %v", err)
	}
}

// TestPolicyContextCanceled asserts that canceling the policy context
// aborts the prompt wait.
func TestPolicyContextCanceled(t *testing.T) {
	// Create the broker policy for a canceled socket takeover.
	b := NewBrokerWithTimeout(5 * time.Second)
	policy := b.MakePolicy("spacewave serve", "/tmp/sock")

	// Run the takeover policy with a cancelable context.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- policy(ctx)
	}()

	// Cancel the takeover policy after its desktop prompt appears.
	waitForPrompt(t, b, time.Second)
	cancel()

	// Require the canceled takeover policy to stop with an error.
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("policy returned nil on canceled context")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("policy did not return on canceled context")
	}
}

// TestSnapshotPromptsBroadcast asserts that snapshotting returns a
// wait channel which closes on state changes.
func TestSnapshotPromptsBroadcast(t *testing.T) {
	// Create the broker policy whose prompt will notify snapshot watchers.
	b := NewBrokerWithTimeout(5 * time.Second)
	policy := b.MakePolicy("spacewave serve", "/tmp/sock")

	// Observe the empty prompt snapshot and capture its change notification.
	initial, waitCh := b.SnapshotPrompts()
	if len(initial) != 0 {
		t.Fatalf("initial prompts non-empty: %d", len(initial))
	}

	// Start a socket takeover to publish a desktop prompt.
	var wg sync.WaitGroup
	wg.Go(func() {
		_ = policy(context.Background())
	})

	// Require the prompt snapshot notification to fire for the new takeover.
	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("wait channel never closed")
	}

	// Verify that the new takeover publishes one pending desktop prompt.
	prompts, _ := b.SnapshotPrompts()
	if len(prompts) != 1 {
		t.Fatalf("expected 1 prompt after policy call, got %d", len(prompts))
	}

	// Deny the published prompt and wait for the takeover policy to finish.
	if err := b.ResolvePrompt(prompts[0].ID, false); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	wg.Wait()
}

// TestReclaimHandoff asserts BeginHandoff + Reclaim coordinate via
// the reclaim channel and clear the handoff state.
func TestReclaimHandoff(t *testing.T) {
	// Create the broker for a listener socket handoff.
	b := NewBrokerWithTimeout(5 * time.Second)

	// Hand off the listener socket to the requesting runtime.
	ch := b.BeginHandoff("spacewave serve", "/tmp/sock")
	state, _ := b.SnapshotHandoff()

	// Verify that the desktop handoff snapshot identifies the active runtime.
	if !state.Active {
		t.Fatalf("handoff state not active after BeginHandoff")
	}
	if state.RequesterName != "spacewave serve" {
		t.Fatalf("handoff requester name: %q", state.RequesterName)
	}

	// Request that the listener reclaim its handed off socket.
	if !b.Reclaim() {
		t.Fatalf("Reclaim returned false while handoff active")
	}

	// Require the listener reclaim notification to fire.
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("reclaim channel not closed")
	}

	// Verify that reclaim clears the handoff and cannot fire again.
	state, _ = b.SnapshotHandoff()
	if state.Active {
		t.Fatalf("handoff still active after Reclaim")
	}
	if b.Reclaim() {
		t.Fatalf("Reclaim returned true with no active handoff")
	}
}

// TestResolveUnknownPrompt asserts that resolving an unknown prompt
// id returns an error.
func TestResolveUnknownPrompt(t *testing.T) {
	b := NewBrokerWithTimeout(time.Second)
	if err := b.ResolvePrompt("missing", true); err == nil {
		t.Fatalf("expected error for unknown prompt id")
	}
}

// waitForPrompt waits until the broker has a pending prompt.
func waitForPrompt(t *testing.T, b *Broker, timeout time.Duration) Prompt {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		prompts, waitCh := b.SnapshotPrompts()
		if len(prompts) > 0 {
			return prompts[0]
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("prompt did not appear within %s", timeout)
		}
		select {
		case <-waitCh:
		case <-time.After(remaining):
			t.Fatalf("timed out waiting for prompt")
		}
	}
}
