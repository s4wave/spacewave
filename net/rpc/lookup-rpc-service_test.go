package bifrost_rpc

import (
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/sirupsen/logrus"
)

// TestLookupRpcServiceIdleRetention checks reuse, renewal, and final disposal.
func TestLookupRpcServiceIdleRetention(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		name := "unresolved"
		if resolved {
			name = "resolved"
		}
		t.Run(name, func(t *testing.T) {
			// Exercise the real bus with and without an available service.
			tb, err := testbed.NewTestbed(t.Context(), logrus.NewEntry(logrus.New()), testbed.TestbedOpts{NoEcho: true, NoPeer: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)

			// Resolve or register the service lookup for the selected availability case.
			var instance directive.Instance
			var ref directive.Reference
			if resolved {
				t.Cleanup(startMockHandler(t, tb))
				_, instance, ref, err = ExLookupRpcService(t.Context(), tb.Bus, mockServiceID, "", true, nil)
			} else {
				instance, ref, err = tb.Bus.AddDirective(NewLookupRpcService(mockServiceID, ""), nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(ref.Release)

			// Observe disposal of the lookup after its references are released.
			disposed := make(chan struct{})
			t.Cleanup(instance.AddDisposeCallback(func() { close(disposed) }))

			// A gap longer than the old timeout must preserve the lookup.
			ref.Release()
			select {
			case <-disposed:
				t.Fatal("lookup disposed during its idle grace period")
			case <-time.After(250 * time.Millisecond):
			}
			reused, nextRef, err := tb.Bus.AddDirective(NewLookupRpcService(mockServiceID, ""), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(nextRef.Release)
			if reused != instance {
				t.Fatal("equivalent lookup did not reuse the retained instance")
			}

			// A renewed reference cancels the original disposal deadline.
			select {
			case <-disposed:
				t.Fatal("lookup disposed while referenced")
			case <-time.After(time.Second):
			}

			// The final release starts a fresh one-second idle period.
			releasedAt := time.Now()
			nextRef.Release()
			select {
			case <-disposed:
				if time.Since(releasedAt) < time.Second {
					t.Fatal("lookup disposed before one second without references")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("unreferenced lookup was not disposed")
			}
		})
	}
}
