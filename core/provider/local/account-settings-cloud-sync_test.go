package provider_local

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cutil_routine "github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestBuildAccountSettingsSyncOps(t *testing.T) {
	source := &account_settings.AccountSettings{
		DisplayName: "Device A",
		PairedDevices: []*account_settings.PairedDevice{{
			PeerId:      "peer-a",
			DisplayName: "Laptop",
			PairedAt:    10,
		}},
		EntityKeypairs: []*session.EntityKeypair{{
			PeerId:     "kp-a",
			AuthMethod: "passkey",
		}},
	}
	target := &account_settings.AccountSettings{
		DisplayName: "Device B",
		PairedDevices: []*account_settings.PairedDevice{{
			PeerId:      "peer-b",
			DisplayName: "Old Phone",
			PairedAt:    1,
		}},
		EntityKeypairs: []*session.EntityKeypair{{
			PeerId:     "kp-b",
			AuthMethod: "pem",
		}},
	}

	ops, err := buildAccountSettingsSyncOps(source, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 5 {
		t.Fatalf("expected 5 sync ops, got %d", len(ops))
	}
}

func TestAccountSettingsSyncTerminalError(t *testing.T) {
	wrap := func(err error) error {
		// Mirror the routine body, which wraps every failure with context.
		return errors.Wrap(err, "mount cloud account settings")
	}
	permanent := &clouderror.Error{
		StatusCode: http.StatusBadRequest,
		Code:       "invalid_resource_id",
		Retryable:  false,
	}
	unauth := &clouderror.Error{
		StatusCode: http.StatusUnauthorized,
		Code:       "unknown_session",
		Retryable:  false,
	}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"canceled", context.Canceled, false},
		{"transient network", errors.New("dial tcp: timeout"), false},
		{"unauth waits for reauth", wrap(unauth), false},
		{"400 stops the loop", wrap(permanent), true},
		{"insufficient_role stays retriable", wrap(&clouderror.Error{
			StatusCode: http.StatusForbidden,
			Code:       "insufficient_role",
			Retryable:  false,
		}), false},
		{"subscription_required stays retriable", wrap(&clouderror.Error{
			StatusCode: http.StatusPaymentRequired,
			Code:       "subscription_required",
			Retryable:  false,
		}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := accountSettingsSyncTerminalError(tc.err); got != tc.want {
				t.Fatalf("accountSettingsSyncTerminalError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// newAccountSettingsSyncTestRoutine builds a state-routine container wired the
// same way as the account-settings cloud sync in account.go: same-state
// compare, provider backoff, and a state routine which classifies stub sync
// errors through the real terminal predicate. Each invocation of the stub
// reports its call number on calls.
func newAccountSettingsSyncTestRoutine(
	t *testing.T,
	run cutil_routine.StateRoutine[string],
) *cutil_routine.StateRoutineContainer[string] {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	ctr := cutil_routine.NewStateRoutineContainerWithLogger[string](
		func(v1, v2 string) bool { return v1 == v2 },
		logrus.NewEntry(log),
		cutil_routine.WithRetry(providerBackoff),
	)
	ctr.SetStateRoutine(run)
	t.Cleanup(func() { ctr.ClearContext() })
	return ctr
}

// awaitAccountSettingsSyncCall waits a bounded time for the next invocation
// signal instead of sleeping a fixed interval.
func awaitAccountSettingsSyncCall(
	t *testing.T,
	calls <-chan int32,
	want int32,
	timeout time.Duration,
) {
	t.Helper()
	var last int32
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case n := <-calls:
			if n >= want {
				return
			}
			last = n
		case <-deadline.C:
			t.Fatalf("routine invoked %d times so far, want at least %d", last, want)
		}
	}
}

func TestAccountSettingsSyncTerminalStopWiring(t *testing.T) {
	ctx := t.Context()

	t.Run("terminal error stops until link state changes", func(t *testing.T) {
		var calls atomic.Int32
		callCh := make(chan int32, 16)
		hardTerminal := &clouderror.Error{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_resource_id",
			Retryable:  false,
		}
		run := func(_ context.Context, _ string) error {
			n := calls.Add(1)
			select {
			case callCh <- n:
			default:
			}
			err := errors.Wrap(hardTerminal, "mount cloud account settings")
			if accountSettingsSyncTerminalError(err) {
				return nil
			}
			return err
		}
		ctr := newAccountSettingsSyncTestRoutine(t, run)
		ctr.SetState("cloud-account-1")
		ctr.SetContext(ctx, true)

		awaitAccountSettingsSyncCall(t, callCh, 1, 10*time.Second)
		if err := ctr.WaitExited(ctx, false, nil); err != nil {
			t.Fatalf("routine exited with error: %v", err)
		}

		// The routine must not re-enter through retry backoff while the
		// linked account state is unchanged.
		select {
		case n := <-callCh:
			t.Fatalf("routine re-entered after terminal stop: call %d", n)
		case <-time.After(1200 * time.Millisecond):
		}

		// A different linked cloud account restarts the routine.
		if _, changed, _, _ := ctr.SetState("cloud-account-2"); !changed {
			t.Fatal("expected state change")
		}
		awaitAccountSettingsSyncCall(t, callCh, 2, 10*time.Second)
		if err := ctr.WaitExited(ctx, false, nil); err != nil {
			t.Fatalf("routine exited with error after restart: %v", err)
		}
		if got := calls.Load(); got != 2 {
			t.Fatalf("routine ran %d times, want 2", got)
		}
	})

	t.Run("access-gated error keeps retrying and recovers", func(t *testing.T) {
		var calls atomic.Int32
		callCh := make(chan int32, 16)
		gated := &clouderror.Error{
			StatusCode: http.StatusForbidden,
			Code:       "insufficient_role",
			Retryable:  false,
		}
		run := func(_ context.Context, _ string) error {
			n := calls.Add(1)
			select {
			case callCh <- n:
			default:
			}
			if n == 1 {
				err := errors.Wrap(gated, "wait for sync op")
				if accountSettingsSyncTerminalError(err) {
					return nil
				}
				return err
			}
			// The role correction landed; the next attempt succeeds.
			return nil
		}
		ctr := newAccountSettingsSyncTestRoutine(t, run)
		ctr.SetState("cloud-account-1")
		ctr.SetContext(ctx, true)

		// The gated rejection must not stop the loop: the second attempt
		// must start via retry backoff.
		awaitAccountSettingsSyncCall(t, callCh, 2, 10*time.Second)
		if err := ctr.WaitExited(ctx, false, nil); err != nil {
			t.Fatalf("routine exited with error after recovery: %v", err)
		}
		if got := calls.Load(); got != 2 {
			t.Fatalf("routine ran %d times, want 2", got)
		}
	})
}

func TestLoadLinkedCloudAccountID(t *testing.T) {
	ctx := t.Context()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provCtrlRef.Release()

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	localProv := prov.(*Provider)
	accIface, accRel, err := localProv.AccessProviderAccount(
		ctx,
		"local-account-123",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer accRel()

	acc := accIface.(*ProviderAccount)
	if err := acc.writeLinkedCloudAccountID(ctx, "local-session-123", "cloud-account-123"); err != nil {
		t.Fatal(err)
	}
	cloudAccountID, err := acc.loadLinkedCloudAccountID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cloudAccountID != "cloud-account-123" {
		t.Fatalf("expected linked cloud account id, got %q", cloudAccountID)
	}
}

func TestFindLinkedCloudSessionRef(t *testing.T) {
	want := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "cloud-session",
			ProviderId:        "spacewave",
			ProviderAccountId: "cloud-account",
		},
	}
	entries := []*session.SessionListEntry{
		{
			SessionRef: &session.SessionRef{
				ProviderResourceRef: &provider.ProviderResourceRef{
					Id:                "local-session",
					ProviderId:        "local",
					ProviderAccountId: "cloud-account",
				},
			},
		},
		{
			SessionRef: &session.SessionRef{
				ProviderResourceRef: &provider.ProviderResourceRef{
					Id:                "other-cloud-session",
					ProviderId:        "spacewave",
					ProviderAccountId: "other-account",
				},
			},
		},
		{SessionRef: want},
	}

	got := findLinkedCloudSessionRef(entries, "cloud-account")
	if !got.EqualVT(want) {
		t.Fatalf("linked cloud session ref = %v, want %v", got, want)
	}
	if got := findLinkedCloudSessionRef(entries, "missing"); got != nil {
		t.Fatalf("missing linked cloud session ref = %v, want nil", got)
	}
}

func TestWaitForLinkedCloudSessionRef(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	_, sessionCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
		VolumeId: tb.EngineVolumeID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessionCtrlRef.Release()

	ctrl, ctrlRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlRef.Release()

	type result struct {
		ref *session.SessionRef
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		ref, err := waitForLinkedCloudSessionRef(ctx, ctrl, "cloud-account")
		resultCh <- result{ref: ref, err: err}
	}()

	want := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "cloud-session",
			ProviderId:        "spacewave",
			ProviderAccountId: "cloud-account",
		},
	}
	if _, err := ctrl.RegisterSession(ctx, want, nil); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if !got.ref.EqualVT(want) {
			t.Fatalf("linked cloud session ref = %v, want %v", got.ref, want)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
