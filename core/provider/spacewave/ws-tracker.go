package provider_spacewave

import (
	"context"
	"net/url"
	"time"

	ws "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/refcount"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// wsTracker manages a single multiplexed WebSocket connection to the Session DO.
// Multiple cloudSOHost instances register callbacks keyed by so_id.
type wsTracker struct {
	// le is the logger.
	le *logrus.Entry
	// getClient returns the current session client for API calls.
	// The client may initially have a nil key; the tracker retries until ready.
	getClient func() *SessionClient
	// onSessionUnauthenticated is called when a stale session key error is
	// detected (recoverable via reauthentication). Called instead of
	// onAccountWasDeleted for unauthCodes errors.
	onSessionUnauthenticated func()
	// onAccountWasDeleted is called when a non-retryable cloud error is detected.
	onAccountWasDeleted func()
	// onAccountChanged is called when a session_event{type:"account_changed"}
	// is received via the WebSocket, indicating remote account state mutation.
	// The epoch parameter is the account epoch from the event payload.
	onAccountChanged func(epoch uint64)
	// onOrgChanged is called when a session_event{type:"org_changed"} is
	// received, indicating organization membership or state change.
	onOrgChanged func(string)
	// onPendingParticipant is called when a session_event{
	// type:"pending_participant"} is received for an owned shared object.
	onPendingParticipant func(string, string)
	// onMemberSessionChanged is called when a session_event{
	// type:"member_session_added" or "member_session_removed"} is received.
	// Parameters: soID, sessionPeerID, accountID, added (true=added, false=removed).
	onMemberSessionChanged func(soID, sessionPeerID, accountID string, added bool)
	// onSONotify is called when a session_event{type:"so_notify"} is received.
	// Parameters: soID, parsed payload.
	onSONotify func(string, *api.SONotifyEventPayload)
	// onInviteMailbox is called when a session_event{type:"invite_mailbox"} is
	// received. The event carries the full mailbox entry and DO-side
	// updatedAt so the receiver applies the delta without refetching.
	onInviteMailbox func(soID string, entry *api.MailboxEntry, updatedAt int64)
	// onInviteMailboxUpdate is called when a session_event{
	// type:"invite_mailbox_update"} is received. The event carries the full
	// updated mailbox entry and DO-side updatedAt.
	onInviteMailboxUpdate func(soID string, entry *api.MailboxEntry, updatedAt int64)
	// onUpdateAvailable is called when a session_event{type:"update_available"}
	// is received, indicating the release orchestrator has published a new
	// dist config and the launcher should re-fetch immediately.
	onUpdateAvailable func()
	// onCdnRootChanged is called when a session_event{type:"cdn_root_changed"}
	// is received. The so_id is the CDN Space ID whose =root.packedmsg=
	// regenerated; receivers should invalidate any cached root pointer for
	// that Space and re-fetch on the next graph lookup.
	onCdnRootChanged func(spaceID string)
	// onSOListUpdate is called when a so_list_update message is received.
	onSOListUpdate func(*sobject.SharedObjectList)
	// onReconnected is called after a session websocket reconnect completes
	// authentication. Receivers should invalidate caches that rely on
	// event-carried state so a fresh seed covers events missed during the gap.
	onReconnected func()
	// onDormantChanged is called when the tracker enters or exits idle mode.
	// dormant=true means access is gated (subscription_required or rbac_denied).
	onDormantChanged func(dormant bool)
	// accountBcast is the account broadcast, used to wake from subscription-idle.
	accountBcast *broadcast.Broadcast
	// notifyCallbacks maps so_id to so_notify event callback.
	notifyCallbacks map[string]func(*api.SONotifyEventPayload)
	// bcast guards the notifyCallbacks map.
	bcast broadcast.Broadcast
	// dormant is true while the tracker is idling on an idleable cloud error.
	// Only touched by Execute/runWebSocket, which run on a single goroutine.
	dormant bool
	// authenticatedOnce tracks whether at least one authenticated session has
	// completed so later reconnects can fire onReconnected.
	authenticatedOnce bool
	// rc manages the shared websocket lifecycle with retry backoff.
	rc *refcount.RefCount[struct{}]
}

// newWSTracker constructs a new wsTracker.
func newWSTracker(le *logrus.Entry, getClient func() *SessionClient) *wsTracker {
	t := &wsTracker{
		le:              le,
		getClient:       getClient,
		notifyCallbacks: make(map[string]func(*api.SONotifyEventPayload)),
	}
	t.rc = refcount.NewRefCountWithOptions(
		nil,
		true,
		nil,
		nil,
		t.resolve,
		&refcount.Options{
			RetryBackoff: providerBackoff,
			ShouldRetry: func(err error) bool {
				return err != nil &&
					!errors.Is(err, context.Canceled) &&
					!isUnauthCloudError(err) &&
					!isNonRetryableCloudError(err)
			},
		},
	)
	return t
}

// SetContext sets the parent lifecycle context for the shared websocket.
func (t *wsTracker) SetContext(ctx context.Context) {
	_ = t.rc.SetContext(ctx)
}

// ClearContext clears the parent lifecycle context for the shared websocket.
func (t *wsTracker) ClearContext() {
	t.rc.ClearContext()
}

// AddRef adds a shared reference to the websocket lifecycle.
func (t *wsTracker) AddRef() *refcount.Ref[struct{}] {
	return t.rc.AddRef(nil)
}

// resolve runs the shared websocket lifecycle for one refcount epoch. Retry
// backoff is handled by the refcount itself; idleable cloud errors stay in the
// routine so account broadcasts can wake the next dial attempt.
func (t *wsTracker) resolve(ctx context.Context, _ func()) (struct{}, func(), error) {
	return struct{}{}, nil, t.Execute(ctx)
}

// isIdleableCloudError checks if an error should cause the wsTracker to idle
// and wait for account changes rather than treating the account as deleted.
func isIdleableCloudError(err error) bool {
	var ce *cloudError
	if errors.As(err, &ce) {
		switch ce.Code {
		case "subscription_required", "rbac_denied":
			return true
		}
	}
	return false
}

// sessionWebSocketPingInterval is the liveness interval for the session WS.
const sessionWebSocketPingInterval = 15 * time.Second

// Execute runs the wsTracker lifecycle until cancellation or a terminal error.
func (t *wsTracker) Execute(ctx context.Context) error {
	for {
		authenticated, err := t.runWebSocket(ctx, t.authenticatedOnce)
		if authenticated {
			t.authenticatedOnce = true
		}
		if ctx.Err() != nil {
			return context.Canceled
		}
		if err != nil {
			if isIdleableCloudError(err) {
				if !t.dormant {
					t.le.WithError(err).Warn("access denied, entering idle until account changes")
					if t.onDormantChanged != nil {
						t.onDormantChanged(true)
					}
					t.dormant = true
				}
				if err := t.waitForAccountChanged(ctx); err != nil {
					return err
				}
				continue
			}
			if isUnauthCloudError(err) {
				t.le.WithError(err).Warn("session key stale, marking unauthenticated")
				if t.onSessionUnauthenticated != nil {
					t.onSessionUnauthenticated()
				}
				return err
			}
			if isNonRetryableCloudError(err) {
				t.le.WithError(err).Warn("permanent cloud error, marking account deleted")
				if t.onAccountWasDeleted != nil {
					t.onAccountWasDeleted()
				}
				return err
			}
			return err
		}
	}
}

// waitForAccountChanged blocks until the account broadcast fires or the context
// is canceled.
func (t *wsTracker) waitForAccountChanged(ctx context.Context) error {
	if t.accountBcast == nil {
		return errors.New("no account broadcast configured")
	}
	var ch <-chan struct{}
	t.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	select {
	case <-ctx.Done():
		return context.Canceled
	case <-ch:
		return nil
	}
}

// runWebSocket dials the session WS and runs the read loop.
func (t *wsTracker) runWebSocket(ctx context.Context, reconnect bool) (bool, error) {
	client := t.getClient()
	if client == nil || client.priv == nil {
		return false, errors.New("session client not ready")
	}

	// Get a short-lived ticket for WebSocket auth.
	ticket, err := client.GetSessionTicket(ctx)
	if err != nil {
		return false, errors.Wrap(err, "get session ticket")
	}

	// Build WS URL with ticket as query param.
	wsURL := client.baseURL + "/api/session/ws?tk=" + url.QueryEscape(ticket)

	conn, err := dialSessionWS(ctx, wsURL)
	if err != nil {
		return false, err
	}
	defer conn.CloseNow()

	// Read the 32-byte challenge from server.
	_, challenge, err := conn.Read(ctx)
	if err != nil {
		return false, errors.Wrap(err, "read challenge")
	}
	if len(challenge) != 32 {
		return false, errors.New("invalid challenge length")
	}

	// Sign ticket+challenge with the session private key.
	ticketBytes := []byte(ticket)
	payload := make([]byte, len(ticketBytes)+32)
	copy(payload, ticketBytes)
	copy(payload[len(ticketBytes):], challenge)

	sig, err := client.priv.Sign(payload)
	if err != nil {
		return false, errors.Wrap(err, "sign challenge")
	}

	// Send the 64-byte signature back as binary.
	if err := conn.Write(ctx, ws.MessageBinary, sig); err != nil {
		return false, errors.Wrap(err, "send challenge response")
	}

	t.le.Debug("session websocket authenticated")

	if t.dormant {
		t.dormant = false
		if t.onDormantChanged != nil {
			t.onDormantChanged(false)
		}
	}

	if reconnect && t.onReconnected != nil {
		t.onReconnected()
	}

	pingRoutine := routine.NewRoutineContainer()
	pingRoutine.SetRoutine(func(rctx context.Context) error {
		return runWebSocketPing(rctx, conn, sessionWebSocketPingInterval)
	})
	pingRoutine.SetContext(ctx, false)
	defer pingRoutine.ClearContext()

	// Read loop.
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return true, errors.Wrap(err, "read session websocket message")
		}

		msg := &api.SessionMessage{}
		if err := msg.UnmarshalVT(data); err != nil {
			t.le.WithError(err).Warn("failed to unmarshal session message")
			continue
		}

		switch {
		case msg.GetSoListUpdate() != nil:
			t.le.Debug("received so list update via session ws")
			if t.onSOListUpdate != nil {
				t.onSOListUpdate(msg.GetSoListUpdate())
			}

		case msg.GetSessionEvent() != nil:
			evt := msg.GetSessionEvent()
			t.le.WithField("event-type", evt.GetType()).
				WithField("so-id", evt.GetSoId()).
				Debug("received session event")
			if evt.GetType() == "account_changed" && t.onAccountChanged != nil {
				epoch := parseEpochFromPayload(evt.GetPayload())
				t.onAccountChanged(epoch)
			}
			if evt.GetType() == "org_changed" && t.onOrgChanged != nil {
				t.onOrgChanged(parseOrgIDFromPayload(evt.GetPayload()))
			}
			if evt.GetType() == "pending_participant" && t.onPendingParticipant != nil {
				accountID := parseAccountIDFromPayload(evt.GetPayload())
				if accountID != "" {
					t.onPendingParticipant(evt.GetSoId(), accountID)
				}
			}
			if (evt.GetType() == "member_session_added" || evt.GetType() == "member_session_removed") && t.onMemberSessionChanged != nil {
				var payload api.MemberSessionChangedPayload
				if err := payload.UnmarshalVT(evt.GetPayload()); err == nil && payload.GetSessionPeerId() != "" {
					t.onMemberSessionChanged(evt.GetSoId(), payload.GetSessionPeerId(), payload.GetAccountId(), evt.GetType() == "member_session_added")
				}
			}
			if evt.GetType() == "so_notify" {
				soID := evt.GetSoId()
				payload, ok := parseSONotifyEventPayload(evt.GetPayload())
				var notifyCb func(*api.SONotifyEventPayload)
				if ok && soID != "" {
					t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
						notifyCb = t.notifyCallbacks[soID]
					})
				}
				if notifyCb != nil {
					notifyCb(payload)
				}
				if t.onSONotify != nil {
					t.onSONotify(soID, payload)
				}
			}
			if evt.GetType() == "invite_mailbox" && t.onInviteMailbox != nil {
				entry, updatedAt, ok := parseInviteMailboxEventPayload(evt.GetPayload())
				if ok {
					t.onInviteMailbox(evt.GetSoId(), entry, updatedAt)
				}
			}
			if evt.GetType() == "invite_mailbox_update" && t.onInviteMailboxUpdate != nil {
				entry, updatedAt, ok := parseInviteMailboxEventPayload(evt.GetPayload())
				if ok {
					t.onInviteMailboxUpdate(evt.GetSoId(), entry, updatedAt)
				}
			}
			if evt.GetType() == "update_available" && t.onUpdateAvailable != nil {
				t.onUpdateAvailable()
			}
			if evt.GetType() == "cdn_root_changed" && t.onCdnRootChanged != nil {
				t.onCdnRootChanged(evt.GetSoId())
			}
		}
	}
}

// runWebSocketPing pings the websocket until the context is canceled.
func runWebSocketPing(ctx context.Context, conn *ws.Conn, interval time.Duration) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}

		if err := conn.Ping(ctx); err != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			return errors.Wrap(err, "ping websocket")
		}
	}
}

// RegisterNotifyCallback registers a so_notify event callback for a so_id.
// The callback receives the parsed SONotifyEventPayload, including any inline
// SOStateMessage delta or snapshot the cloud attached to the event.
func (t *wsTracker) RegisterNotifyCallback(soID string, cb func(*api.SONotifyEventPayload)) {
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		t.notifyCallbacks[soID] = cb
	})
}

// UnregisterNotifyCallback removes the so_notify callback for a so_id.
func (t *wsTracker) UnregisterNotifyCallback(soID string) {
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		delete(t.notifyCallbacks, soID)
	})
}

// parseEpochFromPayload extracts the epoch number from an account_changed
// event payload.
// Returns 0 if the payload cannot be parsed.
func parseEpochFromPayload(payload []byte) uint64 {
	if len(payload) == 0 {
		return 0
	}
	var p api.AccountChangedPayload
	if err := p.UnmarshalVT(payload); err != nil {
		return 0
	}
	return p.GetEpoch()
}

// parseOrgIDFromPayload extracts the org ID from an org_changed payload.
func parseOrgIDFromPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var p api.OrgChangedPayload
	if err := p.UnmarshalVT(payload); err != nil {
		return ""
	}
	return p.GetOrgId()
}

// parseAccountIDFromPayload extracts the account ID from a pending_participant
// payload.
func parseAccountIDFromPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var p api.PendingParticipantPayload
	if err := p.UnmarshalVT(payload); err != nil {
		return ""
	}
	return p.GetAccountId()
}

// parseSONotifyEventPayload decodes the proto-encoded SONotifyEventPayload
// attached to so_notify session events. Returns false when the payload is
// empty or fails to parse; the receiver should treat that as a bare notify
// without inline state.
func parseSONotifyEventPayload(payload []byte) (*api.SONotifyEventPayload, bool) {
	if len(payload) == 0 {
		return nil, false
	}
	var p api.SONotifyEventPayload
	if err := p.UnmarshalVT(payload); err != nil {
		return nil, false
	}
	return &p, true
}

// parseInviteMailboxEventPayload decodes the proto-encoded
// InviteMailboxEventPayload attached to invite_mailbox and
// invite_mailbox_update session events. Returns false if the payload cannot
// be parsed or is missing the entry.
func parseInviteMailboxEventPayload(payload []byte) (*api.MailboxEntry, int64, bool) {
	if len(payload) == 0 {
		return nil, 0, false
	}
	var p api.InviteMailboxEventPayload
	if err := p.UnmarshalVT(payload); err != nil {
		return nil, 0, false
	}
	entry := p.GetEntry()
	if entry == nil {
		return nil, 0, false
	}
	return entry, p.GetUpdatedAt(), true
}
