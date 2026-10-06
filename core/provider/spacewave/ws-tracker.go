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
	// onTargetedInvitationsChanged is called when the recipient targeted
	// invitation inbox changed and watchers should reload the durable inbox.
	onTargetedInvitationsChanged func()
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
	// onConnected is called after every session websocket completes
	// authentication, including the first. Events published before the socket
	// subscribed are lost, so receivers refetch state that events would carry.
	onConnected func()
	// onDormantChanged is called when the tracker enters or exits idle mode.
	// dormant=true means access is gated (subscription_required or rbac_denied).
	onDormantChanged func(dormant bool)
	// accountBcast is the account broadcast, used to wake from subscription-idle.
	accountBcast *broadcast.Broadcast
	// notifyCallbacks maps so_id to so_notify event callback.
	notifyCallbacks map[string]func(*api.SONotifyEventPayload)
	// bstoreNonceCallbacks maps block-store resource ids to nonce callbacks.
	bstoreNonceCallbacks map[string]func(uint64)
	// bcast guards the callback maps.
	bcast broadcast.Broadcast
	// dormant is true while the tracker is idling on an idleable cloud error.
	// Only touched by Execute/runWebSocket, which run on a single goroutine.
	dormant bool
	// rc manages the shared websocket lifecycle with retry backoff.
	rc *refcount.RefCount[struct{}]
}

// newWSTracker constructs a new wsTracker.
func newWSTracker(le *logrus.Entry, getClient func() *SessionClient) *wsTracker {
	t := &wsTracker{
		le:                   le,
		getClient:            getClient,
		notifyCallbacks:      make(map[string]func(*api.SONotifyEventPayload)),
		bstoreNonceCallbacks: make(map[string]func(uint64)),
	}
	t.rc = refcount.NewRefCountWithOptions(
		nil,
		true,
		nil,
		nil,
		t.resolve,
		&refcount.Options{
			RetryBackoff: providerBackoff,
			ShouldRetry:  t.shouldRetry,
		},
	)
	return t
}

// shouldRetry reports whether the shared websocket retries after err. It logs
// every give-up except cancellation, since the daemon receives no cloud push
// events until the tracker restarts.
func (t *wsTracker) shouldRetry(err error) bool {
	// Stop quietly on success or cancellation, and loudly on a final error.
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if isUnauthCloudError(err) || isNonRetryableCloudError(err) {
		t.le.WithError(err).Warn("session websocket stopped retrying")
		return false
	}
	return true
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

// sessionWebSocketPingTimeout bounds the wait for one pong on the session WS.
const sessionWebSocketPingTimeout = 10 * time.Second

// Execute runs the wsTracker lifecycle until cancellation or a terminal error.
func (t *wsTracker) Execute(ctx context.Context) error {
	for {
		err := t.runWebSocket(ctx)
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
			if isAccountDeletedCloudError(err) {
				t.le.WithError(err).Warn("account deleted, tearing down account state")
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
func (t *wsTracker) runWebSocket(ctx context.Context) error {
	// Require a ready session client with its signing key.
	client := t.getClient()
	if client == nil || client.priv == nil {
		return errors.New("session client not ready")
	}

	// Get a short-lived ticket for WebSocket auth.
	ticket, err := client.GetSessionTicket(ctx)
	if err != nil {
		return errors.Wrap(err, "get session ticket")
	}

	// Build WS URL with ticket as query param. The endpoint may be the
	// serving origin "/", so join rather than concatenate.
	wsPath, err := url.JoinPath(client.baseURL, "/api/session/ws")
	if err != nil {
		return errors.Wrap(err, "build session websocket URL")
	}
	wsURL := wsPath + "?tk=" + url.QueryEscape(ticket)

	// Dial the session websocket.
	conn, err := dialSessionWS(ctx, wsURL)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	// Read the 32-byte challenge from server.
	_, challenge, err := conn.Read(ctx)
	if err != nil {
		return errors.Wrap(err, "read challenge")
	}
	if len(challenge) != 32 {
		return errors.New("invalid challenge length")
	}

	// Sign ticket+challenge with the session private key.
	ticketBytes := []byte(ticket)
	payload := make([]byte, len(ticketBytes)+32)
	copy(payload, ticketBytes)
	copy(payload[len(ticketBytes):], challenge)
	sig, err := client.priv.Sign(payload)
	if err != nil {
		return errors.Wrap(err, "sign challenge")
	}

	// Send the 64-byte signature back as binary.
	if err := conn.Write(ctx, ws.MessageBinary, sig); err != nil {
		return errors.Wrap(err, "send challenge response")
	}
	t.le.Debug("session websocket authenticated")

	// An authenticated connection ends any dormant state.
	if t.dormant {
		t.dormant = false
		if t.onDormantChanged != nil {
			t.onDormantChanged(false)
		}
	}

	// Let the owner resync state published before this socket subscribed.
	if t.onConnected != nil {
		t.onConnected()
	}

	// Keep the connection alive with pings for its lifetime.
	pingRoutine := routine.NewRoutineContainer()
	pingRoutine.SetRoutine(func(rctx context.Context) error {
		return runWebSocketPing(rctx, conn, sessionWebSocketPingInterval, sessionWebSocketPingTimeout)
	})
	pingRoutine.SetContext(ctx, false)
	defer pingRoutine.ClearContext()

	// Read loop.
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return errors.Wrap(err, "read session websocket message")
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
			if evt.GetType() == "bstore_nonce" {
				resourceID, nonce, ok := parseBlockStoreNonceEventPayload(evt.GetPayload())
				if resourceID == "" {
					resourceID = evt.GetSoId()
				}
				var nonceCb func(uint64)
				if ok && resourceID != "" {
					t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
						nonceCb = t.bstoreNonceCallbacks[resourceID]
					})
				}
				if nonceCb != nil {
					nonceCb(nonce)
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
			if evt.GetType() == "targeted_invitations_changed" && t.onTargetedInvitationsChanged != nil {
				t.onTargetedInvitationsChanged()
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
//
// A ping that fails or receives no pong within timeout closes the connection,
// which fails the read loop so the tracker reconnects.
func runWebSocketPing(ctx context.Context, conn *ws.Conn, interval, timeout time.Duration) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}

		pingCtx, pingCancel := context.WithTimeout(ctx, timeout)
		err := conn.Ping(pingCtx)
		pingCancel()
		if err != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			_ = conn.CloseNow()
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

// RegisterBlockStoreNonceCallback registers a bstore_nonce callback for a resource id.
func (t *wsTracker) RegisterBlockStoreNonceCallback(resourceID string, cb func(uint64)) {
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		t.bstoreNonceCallbacks[resourceID] = cb
	})
}

// UnregisterBlockStoreNonceCallback removes a bstore_nonce callback for a resource id.
func (t *wsTracker) UnregisterBlockStoreNonceCallback(resourceID string) {
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		delete(t.bstoreNonceCallbacks, resourceID)
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

// parseBlockStoreNonceEventPayload decodes the proto-encoded
// BlockStoreNonceEventPayload attached to bstore_nonce session events.
func parseBlockStoreNonceEventPayload(payload []byte) (string, uint64, bool) {
	if len(payload) == 0 {
		return "", 0, false
	}
	var p api.BlockStoreNonceEventPayload
	if err := p.UnmarshalVT(payload); err != nil {
		return "", 0, false
	}
	return p.GetResourceId(), p.GetNonce(), true
}

// parseInviteMailboxEventPayload decodes the proto-encoded
// InviteMailboxEventPayload attached to invite_mailbox and
// invite_mailbox_update session events. Returns false if the payload cannot
// be parsed or is missing the entry.
func parseInviteMailboxEventPayload(payload []byte) (*api.MailboxEntry, int64, bool) {
	// Decode the payload and require its entry.
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
