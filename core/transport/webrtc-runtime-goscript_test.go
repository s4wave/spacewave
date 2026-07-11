//go:build goscript

package transport

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cbc "github.com/aperturerobotics/controllerbus/core"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

func TestSessionTransportGoScriptStartsSignalingBeforeReady(t *testing.T) {
	requested := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/signal/ticket" {
			http.NotFound(w, r)
			return
		}
		requested <- struct{}{}
		http.Error(w, "test rejection", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.New().WithField("test", t.Name())
	parentBus, _, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	priv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	st, err := NewSessionTransport(le, parentBus, priv, srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- st.Execute(ctx)
	}()

	select {
	case <-requested:
	case <-st.Ready():
		t.Fatal("GoScript session transport reported ready without starting signaling")
	case err := <-done:
		t.Fatalf("session transport exited before requesting a signal ticket: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
