package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/transport/signaling"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestSignalURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "relative root",
			base: "/",
			want: "/api/signal/ticket",
		},
		{
			name: "absolute without trailing slash",
			base: "https://cloud.example",
			want: "https://cloud.example/api/signal/ticket",
		},
		{
			name: "absolute with trailing slash",
			base: "https://cloud.example/",
			want: "https://cloud.example/api/signal/ticket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := signalURL(tt.base, "/api/signal/ticket")
			if err != nil {
				t.Fatal(err)
			}
			if got := u.String(); got != tt.want {
				t.Fatalf("signalURL(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

func TestSignalWebSocketURL(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		ticket string
		want   string
	}{
		{
			name:   "relative root",
			base:   "/",
			ticket: "ticket /?&",
			want:   "/api/signal/ws?tk=ticket+%2F%3F%26",
		},
		{
			name:   "absolute without trailing slash",
			base:   "https://cloud.example",
			ticket: "ticket",
			want:   "wss://cloud.example/api/signal/ws?tk=ticket",
		},
		{
			name:   "absolute with trailing slash",
			base:   "https://cloud.example/",
			ticket: "ticket",
			want:   "wss://cloud.example/api/signal/ws?tk=ticket",
		},
		{
			name:   "http uses ws",
			base:   "http://cloud.example",
			ticket: "ticket",
			want:   "ws://cloud.example/api/signal/ws?tk=ticket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := signalWebSocketURL(tt.base, tt.ticket)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("signalWebSocketURL(%q, %q) = %q, want %q", tt.base, tt.ticket, got, tt.want)
			}
		})
	}
}

// TestAcquireSignalTicketSkewedClock checks that a device whose clock runs
// outside the signaling server's window is refused once, then signs at the
// server's time from the refusal's Date header and gets its ticket, and that
// later tickets succeed at the first request.
func TestAcquireSignalTicketSkewedClock(t *testing.T) {
	// Serve real signaling, recording the status of every ticket request.
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	const env = "test-signaling"
	service := signaling.New(logrus.NewEntry(logrus.New()), func(context.Context, peer.ID) bool {
		return true
	}, env)
	var mtx sync.Mutex
	var statuses []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		service.ServeTicket(rec, r)
		mtx.Lock()
		statuses = append(statuses, rec.status)
		mtx.Unlock()
	}))
	t.Cleanup(server.Close)
	takeStatuses := func() []int {
		mtx.Lock()
		defer mtx.Unlock()
		got := statuses
		statuses = nil
		return got
	}

	// Run the device's clock 11 minutes 34 seconds slow.
	clock := &serverClock{local: func() time.Time {
		return time.Now().Add(-(11*time.Minute + 34*time.Second))
	}}
	if _, err := acquireSignalTicket(t.Context(), server.URL, priv, pid, env, clock); err != nil {
		t.Fatalf("first ticket: %v", err)
	}
	if got, want := takeStatuses(), []int{http.StatusForbidden, http.StatusOK}; !slices.Equal(got, want) {
		t.Fatalf("first ticket statuses = %v, want %v", got, want)
	}
	if _, err := acquireSignalTicket(t.Context(), server.URL, priv, pid, env, clock); err != nil {
		t.Fatalf("second ticket: %v", err)
	}
	if got, want := takeStatuses(), []int{http.StatusOK}; !slices.Equal(got, want) {
		t.Fatalf("second ticket statuses = %v, want %v", got, want)
	}
}

// statusWriter records the status a handler writes.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
