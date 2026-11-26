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
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// wsEvents receives the account-level events of the Session websocket.
//
// The tracker calls each method synchronously on its read goroutine, in the
// order the events arrive, so a method that blocks delays every later event
// and every Space. Each method records its event or hands the work to the
// routine that owns it, and returns without waiting on the network. connected
// runs before the first event of each connection.
type wsEvents interface {
	// connected runs after every session websocket completes authentication,
	// including the first. Events published before the socket subscribed are
	// lost, so the receiver refetches state that events would carry.
	connected()
	// dormantChanged runs when the tracker enters or exits idle mode.
	// dormant=true means access is gated (subscription_required or rbac_denied).
	dormantChanged(dormant bool)
	// sessionUnauthenticated runs when the session key is stale, which
	// reauthentication recovers.
	sessionUnauthenticated()
	// accountWasDeleted runs when the cloud reports the account deleted.
	accountWasDeleted()
	// accountChanged runs on account_changed with the event's account epoch,
	// or 0 when the payload carries none.
	accountChanged(epoch uint64)
	// orgChanged runs on org_changed with the organization id.
	orgChanged(orgID string)
	// pendingParticipant runs on pending_participant for an owned shared object.
	pendingParticipant(soID, accountID string)
	// memberSessionChanged runs on member_session_added (added=true) and
	// member_session_removed (added=false).
	memberSessionChanged(soID, sessionPeerID, accountID string, added bool)
	// soNotify runs on so_notify after the shared object's own soEvents. The
	// payload is nil when the event carried none or it failed to parse.
	soNotify(soID string, payload *api.SONotifyEventPayload)
	// soListUpdate runs on each so_list_update message.
	soListUpdate(list *sobject.SharedObjectList)
	// inviteMailbox runs on invite_mailbox with the full entry and the DO-side
	// updatedAt, so the receiver applies the delta without refetching.
	inviteMailbox(soID string, entry *api.MailboxEntry, updatedAt int64)
	// inviteMailboxUpdate runs on invite_mailbox_update with the full updated
	// entry and the DO-side updatedAt.
	inviteMailboxUpdate(soID string, entry *api.MailboxEntry, updatedAt int64)
	// targetedInvitationsChanged runs when the recipient's targeted invitation
	// inbox changed and watchers should reload it.
	targetedInvitationsChanged()
	// updateAvailable runs when the release orchestrator published a new dist
	// config and the launcher should re-fetch it.
	updateAvailable()
	// cdnRootChanged runs when the CDN Space's root.packedmsg regenerated; the
	// receiver invalidates any cached root pointer for that Space.
	cdnRootChanged(spaceID string)
}

// soEvents receives one shared object's events from the Session websocket.
//
// The tracker calls each method on its read goroutine, so each records the
// event for the shared object's own worker and returns in constant time.
type soEvents interface {
	// connected runs after every session websocket completes authentication,
	// before the connection's first event. Events published while no socket
	// was subscribed are lost, so the receiver resyncs.
	connected()
	// soNotify runs on so_notify for the shared object with its parsed payload.
	soNotify(payload *api.SONotifyEventPayload)
}

// wsTracker manages a single multiplexed WebSocket connection to the Session DO.
// Multiple cloudSOHost instances register soEvents keyed by so_id.
type wsTracker struct {
	// le is the logger.
	le *logrus.Entry
	// getClient returns the current session client for API calls, or nil in the signing gap.
	getClient func() *SessionClient
	// accountBcast is the account broadcast, used to wake from subscription-idle.
	accountBcast *broadcast.Broadcast
	// events receives the account-level events.
	events wsEvents
	// soEvents maps so_id to the shared object's event receiver.
	soEvents map[string]soEvents
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

// newWSTracker constructs a new wsTracker that delivers account-level events
// to events and idles on accountBcast while cloud access is gated.
func newWSTracker(
	le *logrus.Entry,
	getClient func() *SessionClient,
	accountBcast *broadcast.Broadcast,
	events wsEvents,
) *wsTracker {
	t := &wsTracker{
		le:                   le,
		getClient:            getClient,
		accountBcast:         accountBcast,
		events:               events,
		soEvents:             make(map[string]soEvents),
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
	if clouderror.IsUnauth(err) || clouderror.IsNonRetryable(err) {
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
		// Reconnect once the current signer is ready.
		err := t.runWebSocket(ctx)
		if ctx.Err() != nil {
			return context.Canceled
		}
		if errors.Is(err, ErrSigningUnavailable) {
			if err := t.waitForSigner(ctx); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			if isIdleableCloudError(err) {
				if !t.dormant {
					t.le.WithError(err).Warn("access denied, entering idle until account changes")
					t.events.dormantChanged(true)
					t.dormant = true
				}
				if err := t.waitForAccountChanged(ctx); err != nil {
					return err
				}
				continue
			}
			if clouderror.IsUnauth(err) {
				t.le.WithError(err).Warn("session key stale, marking unauthenticated")
				t.events.sessionUnauthenticated()
				return err
			}
			if clouderror.IsAccountDeleted(err) {
				t.le.WithError(err).Warn("account deleted, tearing down account state")
				t.events.accountWasDeleted()
				return err
			}
			return err
		}
	}
}

// waitForSigner waits for a live signer without missing an install between the check and wait.
func (t *wsTracker) waitForSigner(ctx context.Context) error {
	for {
		// Capture the notification before resolving the slot outside its lock.
		var changed <-chan struct{}
		t.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			changed = getWaitCh()
		})
		if t.getClient().GetSessionSigner().CanSign() {
			return nil
		}

		// Wait until the slot changes or the tracker stops.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

// waitForAccountChanged blocks until the account broadcast fires or the context
// is canceled.
func (t *wsTracker) waitForAccountChanged(ctx context.Context) error {
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
func (t *wsTracker) runWebSocket(ctx context.Context) (retErr error) {
	// Require a ready session client with its signing key.
	client := t.getClient()
	if !client.GetSessionSigner().CanSign() {
		return ErrSigningUnavailable
	}

	// End this connection when the signer it authenticated with is revoked.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	defer func() {
		if errors.Is(context.Cause(ctx), ErrSigningUnavailable) {
			retErr = ErrSigningUnavailable
		}
	}()
	go func() {
		select {
		case <-client.GetSessionSigner().Done():
			cancel(ErrSigningUnavailable)
		case <-ctx.Done():
		}
	}()

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
	sig, err := client.GetSessionSigner().Sign(ctx, payload)
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
		t.events.dormantChanged(false)
	}

	// Let the owners resync state published before this socket subscribed.
	t.events.connected()
	for _, so := range t.snapshotSOEvents() {
		so.connected()
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

		// Decode the next typed session event.
		msg := &api.SessionMessage{}
		if err := msg.UnmarshalVT(data); err != nil {
			t.le.WithError(err).Warn("failed to unmarshal session message")
			continue
		}

		// Dispatch the event to its account or SharedObject owner.
		switch {
		case msg.GetSoListUpdate() != nil:
			t.le.Debug("received so list update via session ws")
			t.events.soListUpdate(msg.GetSoListUpdate())

		case msg.GetSessionEvent() != nil:
			evt := msg.GetSessionEvent()
			t.le.WithField("event-type", evt.GetType()).
				WithField("so-id", evt.GetSoId()).
				Debug("received session event")
			if evt.GetType() == "account_changed" {
				t.events.accountChanged(parseEpochFromPayload(evt.GetPayload()))
			}
			if evt.GetType() == "org_changed" {
				t.events.orgChanged(parseOrgIDFromPayload(evt.GetPayload()))
			}
			if evt.GetType() == "pending_participant" {
				accountID := parseAccountIDFromPayload(evt.GetPayload())
				if accountID != "" {
					t.events.pendingParticipant(evt.GetSoId(), accountID)
				}
			}
			if evt.GetType() == "member_session_added" || evt.GetType() == "member_session_removed" {
				var payload api.MemberSessionChangedPayload
				if err := payload.UnmarshalVT(evt.GetPayload()); err == nil && payload.GetSessionPeerId() != "" {
					t.events.memberSessionChanged(evt.GetSoId(), payload.GetSessionPeerId(), payload.GetAccountId(), evt.GetType() == "member_session_added")
				}
			}
			if evt.GetType() == "so_notify" {
				soID := evt.GetSoId()
				payload, ok := parseSONotifyEventPayload(evt.GetPayload())
				var so soEvents
				if ok && soID != "" {
					t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
						so = t.soEvents[soID]
					})
				}
				if so != nil {
					so.soNotify(payload)
				}
				t.events.soNotify(soID, payload)
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
			if evt.GetType() == "invite_mailbox" {
				entry, updatedAt, ok := parseInviteMailboxEventPayload(evt.GetPayload())
				if ok {
					t.events.inviteMailbox(evt.GetSoId(), entry, updatedAt)
				}
			}
			if evt.GetType() == "invite_mailbox_update" {
				entry, updatedAt, ok := parseInviteMailboxEventPayload(evt.GetPayload())
				if ok {
					t.events.inviteMailboxUpdate(evt.GetSoId(), entry, updatedAt)
				}
			}
			if evt.GetType() == "targeted_invitations_changed" {
				t.events.targetedInvitationsChanged()
			}
			if evt.GetType() == "update_available" {
				t.events.updateAvailable()
			}
			if evt.GetType() == "cdn_root_changed" {
				t.events.cdnRootChanged(evt.GetSoId())
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
		//gostyle:timer deadline
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

// RegisterSOEvents registers the event receiver of a so_id. It receives the
// parsed SONotifyEventPayload, including any inline SOStateMessage delta or
// snapshot the cloud attached, and each connection's start.
func (t *wsTracker) RegisterSOEvents(soID string, events soEvents) {
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		t.soEvents[soID] = events
	})
}

// UnregisterSOEvents removes the event receiver of a so_id.
func (t *wsTracker) UnregisterSOEvents(soID string) {
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		delete(t.soEvents, soID)
	})
}

// snapshotSOEvents returns the registered shared object event receivers.
func (t *wsTracker) snapshotSOEvents() []soEvents {
	var out []soEvents
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		out = make([]soEvents, 0, len(t.soEvents))
		for _, so := range t.soEvents {
			out = append(out, so)
		}
	})
	return out
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
