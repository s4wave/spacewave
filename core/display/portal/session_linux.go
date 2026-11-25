// Package portal opens a KDE RemoteDesktop session for an authorized local display.
package portal

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/util/csync"
	"github.com/godbus/dbus/v5"
)

const (
	// AppID identifies the unsandboxed display capture app to the portal.
	AppID = "org.s4wave.Spacewave.DisplayCapture"
	// desktop is the public portal service on the user's session bus.
	desktop = "org.freedesktop.portal.Desktop"
	// desktopPath serves the RemoteDesktop, ScreenCast and host Registry interfaces.
	desktopPath = "/org/freedesktop/portal/desktop"
	// remoteDesktop creates the session and grants its input devices.
	remoteDesktop = "org.freedesktop.portal.RemoteDesktop"
	// screenCast selects a monitor and exports the restricted PipeWire remote.
	screenCast = "org.freedesktop.portal.ScreenCast"
	// requestInterface carries the terminal response for each portal request.
	requestInterface = "org.freedesktop.portal.Request"
	// sessionInterface controls the lifetime shared by capture and input.
	sessionInterface = "org.freedesktop.portal.Session"
	// permissions identifies the per-user Grant store KDE checks on Start.
	permissions = "org.freedesktop.impl.portal.PermissionStore"
	// permissionPath serves the permission store's methods and change signals.
	permissionPath = "/org/freedesktop/impl/portal/PermissionStore"
	// permissionTable is KDE's table for preauthorized remote-desktop apps.
	permissionTable = "kde-authorized"
	// permissionID names the remote-desktop row in that table.
	permissionID = "remote-desktop"
	// startDeadline bounds Start if KDE falls back to a consent dialog.
	startDeadline = 4 * time.Second
	// lookupDeadline bounds a permission read when the store stops responding.
	lookupDeadline = 2 * time.Second
	// closeDeadline bounds a Close call before the bus connection is released.
	closeDeadline = time.Second
	// inputDevices requests the keyboard and pointer for this display session.
	inputDevices = uint32(3)
)

// Session owns the portal identity and lifetime of both screen capture and input.
type Session struct {
	// ctx ends on revocation, Session.Closed, bus disconnect or explicit close.
	ctx context.Context
	// cancel records why this session can no longer capture or accept input.
	cancel context.CancelCauseFunc
	// conn is private to this capture so portal identity and cleanup share a peer.
	conn *dbus.Conn
	// owner identifies the portal sender on the private connection.
	owner string
	// path is the session object predicted from the connection name and token.
	path dbus.ObjectPath
	// requestRoot prefixes the request objects owned by this connection.
	requestRoot string
	// watched closes when the signal receiver has detached from the connection.
	watched chan struct{}
	// nodeID identifies the chosen screen on the restricted PipeWire remote.
	nodeID uint32
	// width is the stream's logical width in compositor coordinates.
	width int32
	// height is the stream's logical height in compositor coordinates.
	height int32
	// inputMtx orders all motion and held-state transitions on the session.
	inputMtx csync.Mutex
	// pressed counts viewers holding each key or button under inputMtx.
	pressed map[inputCode]int
	// startedAt is the time the caller issued the portal Start request.
	startedAt time.Time
}

// Open starts a combined ScreenCast and RemoteDesktop session on the user's bus.
// The caller closes the returned session to release its capture and input.
func Open(ctx context.Context) (_ *Session, err error) {
	// Register on a new connection before any capture portal method uses it.
	conn, owner, err := connectPortal(ctx)
	if err != nil {
		return nil, err
	}

	// Derive this peer's portal paths before sending any request.
	ctx, cancel := context.WithCancelCause(ctx)
	sender := strings.ReplaceAll(strings.TrimPrefix(conn.Names()[0], ":"), ".", "_")
	s := &Session{
		ctx: ctx, cancel: cancel, conn: conn, owner: owner,
		path:        dbus.ObjectPath("/org/freedesktop/portal/desktop/session/" + sender + "/display"),
		requestRoot: "/org/freedesktop/portal/desktop/request/" + sender,
		watched:     make(chan struct{}),
	}

	// Join the session watcher if setup fails before the source takes ownership.
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	go s.watch(signals)
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()

	// Subscribe before checking the grant so revocation also reaches a pending Start.
	if err := s.subscribe(); err != nil {
		return nil, err
	}
	if err := s.checkGrant(); err != nil {
		return nil, err
	}

	// Prepare the session and select one monitor plus the pointer and keyboard.
	if err := s.prepare(); err != nil {
		return nil, err
	}
	if err := s.start(); err != nil {
		return nil, err
	}
	return s, nil
}

// Context returns the common lifetime for frame capture and input calls.
func (s *Session) Context() context.Context {
	return s.ctx
}

// OpenPipeWireRemote returns an owned descriptor and the node to select on it.
func (s *Session) OpenPipeWireRemote() (*os.File, uint32, error) {
	// Keep the call tracked until its reply arrives or the bus reader exits.
	// A cancelled godbus call discards a late reply without closing its FD.
	if cause := context.Cause(s.ctx); cause != nil {
		return nil, 0, cause
	}
	call := s.conn.Object(s.owner, desktopPath).Go(
		screenCast+".OpenPipeWireRemote", 0, nil, s.path, map[string]dbus.Variant{})
	select {
	case <-call.Done:
	case <-s.ctx.Done():
		<-s.watched
		_ = s.conn.Close()
		<-call.Done
	}

	// Take custody of the actual D-Bus Unix FD and close it when capture ended.
	cause := context.Cause(s.ctx)
	if call.Err != nil {
		return nil, 0, errors.Join(cause, call.Err)
	}
	if len(call.Body) != 1 {
		return nil, 0, errors.New("invalid PipeWire remote reply")
	}
	remote, ok := call.Body[0].(dbus.UnixFD)
	if !ok || remote < 0 {
		return nil, 0, errors.New("portal returned no PipeWire descriptor")
	}
	file := os.NewFile(uintptr(remote), "portal PipeWire remote")
	if cause != nil {
		_ = file.Close()
		return nil, 0, cause
	}
	return file, s.nodeID, nil
}

// Close ends capture and input, and joins the bus signal receiver.
func (s *Session) Close() error {
	// Join the watcher's explicit Session.Close before dropping the connection.
	s.cancel(context.Canceled)
	<-s.watched
	return s.conn.Close()
}

// subscribe installs watches on the grant, portal session and request namespace.
func (s *Session) subscribe() error {
	// Listen for exactly this grant and the portal signals for this connection.
	for _, match := range [][]dbus.MatchOption{
		{
			dbus.WithMatchSender(permissions), dbus.WithMatchInterface(permissions),
			dbus.WithMatchObjectPath(permissionPath), dbus.WithMatchMember("Changed"),
			dbus.WithMatchArg(0, permissionTable), dbus.WithMatchArg(1, permissionID),
		},
		{
			dbus.WithMatchSender(s.owner), dbus.WithMatchInterface(sessionInterface),
			dbus.WithMatchObjectPath(s.path), dbus.WithMatchMember("Closed"),
		},
		{
			dbus.WithMatchSender(s.owner), dbus.WithMatchInterface(requestInterface),
			dbus.WithMatchPathNamespace(dbus.ObjectPath(s.requestRoot)), dbus.WithMatchMember("Response"),
		},
		{
			dbus.WithMatchSender("org.freedesktop.DBus"), dbus.WithMatchInterface("org.freedesktop.DBus"),
			dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, desktop),
		},
		{
			dbus.WithMatchSender("org.freedesktop.DBus"), dbus.WithMatchInterface("org.freedesktop.DBus"),
			dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, permissions),
		},
	} {
		if err := s.conn.AddMatchSignalContext(s.ctx, match...); err != nil {
			return err
		}
	}
	return nil
}

// watch ends the source when the grant, portal session or connection is lost.
func (s *Session) watch(signals chan *dbus.Signal) {
	// Release the portal session before publishing monitor completion.
	defer close(s.watched)
	defer s.conn.RemoveSignal(signals)
	defer s.closeObject(s.path, sessionInterface)
	for {
		select {
		case <-s.ctx.Done():
			return
		case event, ok := <-signals:
			if !ok {
				s.cancel(ErrSessionClosed)
				return
			}
			if event.Sender == s.owner && event.Name == sessionInterface+".Closed" && event.Path == s.path {
				s.cancel(ErrSessionClosed)
				return
			}
			if event.Sender == "org.freedesktop.DBus" && event.Name == "org.freedesktop.DBus.NameOwnerChanged" &&
				len(event.Body) == 3 && event.Body[0] == desktop && event.Body[1] == s.owner {
				s.cancel(ErrSessionClosed)
				return
			}
			if event.Sender == "org.freedesktop.DBus" && event.Name == "org.freedesktop.DBus.NameOwnerChanged" &&
				len(event.Body) == 3 && event.Body[0] == permissions && event.Body[1] != "" {
				s.cancel(ErrNotAuthorized)
				return
			}
			if grantWasRevoked(event) {
				s.cancel(ErrNotAuthorized)
				return
			}
		}
	}
}

// checkGrant reads the permission KDE uses to start without a consent dialog.
func (s *Session) checkGrant() error {
	// Require the actual grant before asking the portal to start.
	ctx, cancel := context.WithTimeout(s.ctx, lookupDeadline)
	defer cancel()
	var grants map[string][]string
	var data dbus.Variant
	err := s.conn.Object(permissions, permissionPath).CallWithContext(ctx, permissions+".Lookup", 0,
		permissionTable, permissionID).Store(&grants, &data)
	if cause := context.Cause(s.ctx); cause != nil {
		return cause
	}
	if err != nil || !slices.Contains(grants[AppID], "yes") {
		return errors.Join(ErrNotAuthorized, err)
	}
	return nil
}

// prepare establishes a session without opening a consent dialog.
func (s *Session) prepare() error {
	// Tie the session to the object path already watched for Closed.
	result, err := s.request(s.ctx, "create", remoteDesktop+".CreateSession", map[string]dbus.Variant{
		"session_handle_token": dbus.MakeVariant("display"),
	})
	if err != nil {
		return err
	}
	if handle, ok := result["session_handle"].Value().(string); !ok || handle != string(s.path) {
		return errors.New("portal returned an unexpected session handle")
	}

	// Request keyboard and pointer using the PermissionStore as the only grant.
	if _, err := s.request(s.ctx, "devices", remoteDesktop+".SelectDevices", map[string]dbus.Variant{
		"types":        dbus.MakeVariant(inputDevices),
		"persist_mode": dbus.MakeVariant(uint32(0)),
	}, s.path); err != nil {
		return err
	}

	// Add one monitor to the existing RemoteDesktop session.
	_, err = s.request(s.ctx, "sources", screenCast+".SelectSources", map[string]dbus.Variant{
		"types":    dbus.MakeVariant(uint32(1)),
		"multiple": dbus.MakeVariant(false),
	}, s.path)
	return err
}

// start starts capture with the last read grant.
// KDE checks the grant again inside Start. If it falls back to a dialog, the
// deadline closes the Request and Open closes the Session. The operator can
// provision the grant again after a timeout or a cancelled or rejected reply.
func (s *Session) start() error {
	// Check as close to Start as the portal protocol allows.
	if err := s.checkGrant(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.ctx, startDeadline)
	defer cancel()
	s.startedAt = time.Now()
	response, err := s.request(ctx, "start", remoteDesktop+".Start", map[string]dbus.Variant{}, s.path, "")
	if cause := context.Cause(s.ctx); cause != nil {
		// The portal can emit Closed immediately after rejecting Start.
		if cause == ErrSessionClosed {
			return errors.Join(ErrNotAuthorized, cause)
		}
		return cause
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errRequestDenied) {
		return errors.Join(ErrNotAuthorized, err)
	}
	if err != nil {
		return err
	}

	// Require the input and screen the node asked KDE to grant.
	if devices, ok := response["devices"].Value().(uint32); !ok || devices&inputDevices != inputDevices {
		return ErrNotAuthorized
	}
	var streams []struct {
		NodeID     uint32
		Properties map[string]dbus.Variant
	}
	if err := dbus.Store([]any{response["streams"].Value()}, &streams); err != nil {
		return err
	}
	if len(streams) == 0 || streams[0].NodeID == 0 {
		return errors.New("plasma returned no PipeWire screen node")
	}
	s.nodeID = streams[0].NodeID

	// The portal reports logical dimensions independently of encoded pixels.
	var size struct{ Width, Height int32 }
	if err := dbus.Store([]any{streams[0].Properties["size"].Value()}, &size); err != nil {
		return err
	}
	if size.Width <= 0 || size.Height <= 0 {
		return errors.New("plasma returned no logical screen size")
	}
	s.width, s.height = size.Width, size.Height
	s.pressed = make(map[inputCode]int)
	return nil
}

// request subscribes before the method call and awaits the Response on its handle.
func (s *Session) request(
	ctx context.Context, token, method string, opts map[string]dbus.Variant, args ...any,
) (map[string]dbus.Variant, error) {
	// Own the predictable handle even if the method call is cancelled in flight.
	expected := dbus.ObjectPath(s.requestRoot + "/" + token)
	signals := make(chan *dbus.Signal, 4)
	s.conn.Signal(signals)
	defer s.conn.RemoveSignal(signals)
	opts["handle_token"] = dbus.MakeVariant(token)
	var handle dbus.ObjectPath
	if err := s.conn.Object(s.owner, desktopPath).CallWithContext(ctx, method, 0, append(args, opts)...).Store(&handle); err != nil {
		s.closeObject(expected, requestInterface)
		if cause := context.Cause(ctx); cause != nil {
			return nil, cause
		}
		return nil, err
	}
	if handle != expected {
		s.closeObject(handle, requestInterface)
		return nil, errors.New("portal returned an unexpected request handle")
	}

	// Close a pending request independently of the context that timed it out.
	for {
		select {
		case <-ctx.Done():
			s.closeObject(handle, requestInterface)
			return nil, context.Cause(ctx)
		case event, ok := <-signals:
			if !ok {
				return nil, ErrSessionClosed
			}
			if event.Sender != s.owner || event.Name != requestInterface+".Response" || event.Path != handle {
				continue
			}
			if err := ctx.Err(); err != nil {
				s.closeObject(handle, requestInterface)
				return nil, context.Cause(ctx)
			}
			var code uint32
			var result map[string]dbus.Variant
			if err := dbus.Store(event.Body, &code, &result); err != nil {
				return nil, err
			}
			if code != 0 {
				return nil, errRequestDenied
			}
			return result, nil
		}
	}
}

// closeObject retires a portal request or session even after its caller has ended.
func (s *Session) closeObject(path dbus.ObjectPath, iface string) {
	// A closed connection also releases its portal objects.
	ctx, cancel := context.WithTimeout(context.Background(), closeDeadline)
	defer cancel()
	_ = s.conn.Object(s.owner, path).CallWithContext(ctx, iface+".Close", 0).Err
}

// connectPortal registers the app ID and pins the owner of the public portal name.
func connectPortal(ctx context.Context) (_ *dbus.Conn, owner string, err error) {
	// Give every session its own registry identity and D-Bus object namespace.
	conn, err := dbus.ConnectSessionBus(dbus.WithSignalHandler(dbus.NewSequentialSignalHandler()))
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()

	// Resolve a single portal owner even if the desktop service needs activation.
	bus := conn.BusObject()
	if err := bus.CallWithContext(ctx, "org.freedesktop.DBus.StartServiceByName", 0, desktop, uint32(0)).Err; err != nil {
		return nil, "", err
	}
	if err := bus.CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, desktop).Store(&owner); err != nil {
		return nil, "", err
	}

	// Register before that owner can cache the peer as an anonymous host app.
	object := conn.Object(owner, desktopPath)
	if err := object.CallWithContext(ctx, "org.freedesktop.host.portal.Registry.Register", 0, AppID, map[string]dbus.Variant{}).Err; err != nil {
		return nil, "", errors.Join(ErrNotAuthorized, err)
	}
	return conn, owner, nil
}

// grantWasRevoked reports a change to KDE's grant row that removes this app's yes.
func grantWasRevoked(event *dbus.Signal) bool {
	// Ignore changes elsewhere in the store.
	if event.Name != permissions+".Changed" || event.Path != permissionPath {
		return false
	}
	var table, id string
	var deleted bool
	var data dbus.Variant
	var grants map[string][]string
	if err := dbus.Store(event.Body, &table, &id, &deleted, &data, &grants); err != nil ||
		table != permissionTable || id != permissionID {
		return false
	}
	return deleted || !slices.Contains(grants[AppID], "yes")
}
