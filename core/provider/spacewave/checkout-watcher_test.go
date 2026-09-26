//go:build !js

package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	websocket "github.com/aperturerobotics/go-websocket"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/sirupsen/logrus"
)

// runCheckoutWatcherTest runs the checkout watcher against a server that
// writes frames and then closes normally.
func runCheckoutWatcherTest(t *testing.T, frames ...*api.WsBillingCheckoutServerFrame) (*checkoutWatcher, bool, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		for _, frame := range frames {
			if err := conn.Write(r.Context(), websocket.MessageBinary, mustMarshalVT(t, frame)); err != nil {
				return
			}
		}
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	var completed bool
	w := newCheckoutWatcher(logrus.NewEntry(logrus.New()), func() *SessionClient { return cli }, func() {
		completed = true
	})
	w.SetTicket("ticket")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := w.runWebSocket(ctx, "ticket")
	return w, completed, err
}

// TestCheckoutWatcherNormalCloseIsNotCompleted checks that a normal close
// without a terminal status frame does not complete the checkout.
func TestCheckoutWatcherNormalCloseIsNotCompleted(t *testing.T) {
	w, completed, err := runCheckoutWatcherTest(t)
	if err == nil {
		t.Fatal("expected disconnect error")
	}
	if completed {
		t.Fatal("onCompleted called without a terminal status frame")
	}
	if status := w.GetStatus(); status != "pending" {
		t.Fatalf("status = %q, want pending", status)
	}
}

// TestCheckoutWatcherCompletedFrame checks that a completed status frame
// completes the checkout.
func TestCheckoutWatcherCompletedFrame(t *testing.T) {
	w, completed, err := runCheckoutWatcherTest(t, &api.WsBillingCheckoutServerFrame{
		Body: &api.WsBillingCheckoutServerFrame_Status{
			Status: &api.CheckoutStatusMessage{Type: "checkout_status", Status: "completed"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("onCompleted not called")
	}
	if status := w.GetStatus(); status != "completed" {
		t.Fatalf("status = %q, want completed", status)
	}
}
