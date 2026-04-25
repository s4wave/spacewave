package provider_spacewave

import (
	"context"
	"net/url"
	"strings"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/sirupsen/logrus"
)

// checkoutWatcher manages a single checkout status WebSocket connection
// shared across multiple WatchCheckoutStatus RPC subscribers via refcount.
type checkoutWatcher struct {
	// le is the logger.
	le *logrus.Entry
	// getClient returns the current session client.
	getClient func() *SessionClient
	// onCompleted is called when checkout status becomes "completed".
	onCompleted func()

	// bcast guards ticket and status fields.
	bcast broadcast.Broadcast
	// ticket is the current checkout WS ticket (JWT).
	ticket string
	// status is the current checkout status.
	status string

	// rc manages the WS goroutine lifecycle.
	rc *refcount.RefCount[struct{}]
}

// newCheckoutWatcher creates a new checkoutWatcher.
func newCheckoutWatcher(le *logrus.Entry, getClient func() *SessionClient, onCompleted func()) *checkoutWatcher {
	w := &checkoutWatcher{
		le:          le,
		getClient:   getClient,
		onCompleted: onCompleted,
	}
	//nolint:staticcheck // SetContext is called with the real context during setup.
	w.rc = refcount.NewRefCount(nil, false, nil, nil, w.resolve)
	return w
}

// SetContext sets the parent lifecycle context for the WS goroutine.
func (w *checkoutWatcher) SetContext(ctx context.Context) {
	_ = w.rc.SetContext(ctx)
}

// ClearContext clears the context and stops the WS goroutine.
func (w *checkoutWatcher) ClearContext() {
	w.rc.ClearContext()
}

// SetTicket stores a new checkout ticket and resets status to pending.
func (w *checkoutWatcher) SetTicket(ticket string) {
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		w.ticket = ticket
		w.status = "pending"
		broadcast()
	})
}

// GetStatus returns the current checkout status.
func (w *checkoutWatcher) GetStatus() string {
	var status string
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		status = w.status
	})
	return status
}

// WaitStatus returns the current status and a channel to wait for changes.
func (w *checkoutWatcher) WaitStatus() (<-chan struct{}, string) {
	var ch <-chan struct{}
	var status string
	w.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
		status = w.status
	})
	return ch, status
}

// HasTicket returns true if a checkout ticket has been set.
func (w *checkoutWatcher) HasTicket() bool {
	var has bool
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		has = w.ticket != ""
	})
	return has
}

// AddRef adds a subscriber reference, starting the WS goroutine if needed.
func (w *checkoutWatcher) AddRef() *refcount.Ref[struct{}] {
	return w.rc.AddRef(nil)
}

// resolve is the RefCount resolver that runs the checkout WS goroutine.
func (w *checkoutWatcher) resolve(ctx context.Context, _ func()) (struct{}, func(), error) {
	var ticket string
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ticket = w.ticket
	})
	if ticket == "" {
		return struct{}{}, nil, errors.New("no checkout ticket")
	}

	err := w.runWebSocket(ctx, ticket)
	return struct{}{}, nil, err
}

// runWebSocket dials the checkout WS and reads status messages.
func (w *checkoutWatcher) runWebSocket(ctx context.Context, ticket string) error {
	client := w.getClient()
	if client == nil {
		return errors.New("session client not ready")
	}

	// Build WS URL: replace http(s) with ws(s).
	wsBase := strings.Replace(client.baseURL, "https://", "wss://", 1)
	wsBase = strings.Replace(wsBase, "http://", "ws://", 1)
	wsURL := wsBase + "/api/billing/checkout/ws?tk=" + url.QueryEscape(ticket)

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return errors.Wrap(err, "dial checkout websocket")
	}
	defer conn.CloseNow()

	for {
		_, data, rErr := conn.Read(ctx)
		if rErr != nil {
			// Normal close with 1000 means completed.
			if websocket.CloseStatus(rErr) == 1000 {
				w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
					w.status = "completed"
					broadcast()
				})
				if w.onCompleted != nil {
					w.onCompleted()
				}
				return nil
			}
			return errors.Wrap(rErr, "read checkout websocket")
		}

		var frame api.WsBillingCheckoutServerFrame
		if err := frame.UnmarshalVT(data); err != nil {
			w.le.WithError(err).Warn("failed to unmarshal checkout ws message")
			continue
		}
		body, ok := frame.GetBody().(*api.WsBillingCheckoutServerFrame_Status)
		if !ok || body.Status == nil {
			w.le.Warn("checkout ws message missing status frame")
			continue
		}
		msg := body.Status
		if msg.GetType() == "checkout_status" && msg.GetStatus() != "" {
			status := msg.GetStatus()
			w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				w.status = status
				broadcast()
			})
			if status == "completed" {
				if w.onCompleted != nil {
					w.onCompleted()
				}
				conn.Close(websocket.StatusNormalClosure, "received terminal status")
				return nil
			}
			if status == "expired" {
				conn.Close(websocket.StatusNormalClosure, "received terminal status")
				return nil
			}
		}
	}
}
