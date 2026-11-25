package portal

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// TestPortalStartFailure exercises timeout and rejected replies on a private bus.
func TestPortalStartFailure(t *testing.T) {
	// A fake portal may only own the names on a bus created for this test.
	if os.Getenv("SPACEWAVE_TEST_PRIVATE_PORTAL_FAILURE") != "1" {
		t.Skip("run inside a private D-Bus with SPACEWAVE_TEST_PRIVATE_PORTAL_FAILURE=1")
	}

	// A missing grant never reaches Start. All failed Starts close the session.
	for _, outcome := range []string{"missing grant", "rejected", "cancelled", "timeout"} {
		t.Run(outcome, func(t *testing.T) { checkPortalStartFailure(t, outcome) })
	}
}

// checkPortalStartFailure exercises the public Open path against a D-Bus fixture.
func checkPortalStartFailure(t *testing.T, outcome string) {
	// Own the portal and grant-store names in the test bus.
	t.Helper()
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	for _, name := range []string{desktop, permissions} {
		reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
		if reply != dbus.RequestNameReplyPrimaryOwner || err != nil {
			t.Fatalf("private service %s returned %v and %v", name, reply, err)
		}
	}

	// Pin the start attempt and cleanup calls to the paths the portal supplied.
	started := make(chan struct{}, 1)
	requestClosed := make(chan struct{}, 1)
	sessionClosed := make(chan struct{}, 1)
	export := func(path dbus.ObjectPath, iface string, methods map[string]any) {
		t.Helper()
		if err := conn.ExportMethodTable(methods, path, iface); err != nil {
			t.Error(err)
		}
	}
	reply := func(sender dbus.Sender, options map[string]dbus.Variant, code uint32, result map[string]dbus.Variant) dbus.ObjectPath {
		peer := strings.ReplaceAll(strings.TrimPrefix(string(sender), ":"), ".", "_")
		token := options["handle_token"].Value().(string)
		handle := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + peer + "/" + token)
		if code == 3 {
			export(handle, requestInterface, map[string]any{
				"Close": func() *dbus.Error { requestClosed <- struct{}{}; return nil },
			})
			return handle
		}
		if err := conn.Emit(handle, requestInterface+".Response", code, result); err != nil {
			t.Error(err)
		}
		return handle
	}

	// Emulate only the portal's registration, selection and authorization calls.
	export(desktopPath, "org.freedesktop.host.portal.Registry", map[string]any{
		"Register": func(app string, _ map[string]dbus.Variant) *dbus.Error {
			if app != AppID {
				t.Errorf("registered app %q", app)
			}
			return nil
		},
	})
	export(permissionPath, permissions, map[string]any{
		"Lookup": func(_, _ string) (map[string][]string, dbus.Variant, *dbus.Error) {
			grants := map[string][]string{}
			if outcome != "missing grant" {
				grants[AppID] = []string{"yes"}
			}
			return grants, dbus.MakeVariant(byte(0)), nil
		},
	})
	export(desktopPath, remoteDesktop, map[string]any{
		"CreateSession": func(sender dbus.Sender, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
			peer := strings.ReplaceAll(strings.TrimPrefix(string(sender), ":"), ".", "_")
			path := dbus.ObjectPath("/org/freedesktop/portal/desktop/session/" + peer + "/display")
			export(path, sessionInterface, map[string]any{
				"Close": func() *dbus.Error { sessionClosed <- struct{}{}; return nil },
			})
			return reply(sender, options, 0, map[string]dbus.Variant{"session_handle": dbus.MakeVariant(string(path))}), nil
		},
		"SelectDevices": func(sender dbus.Sender, _ dbus.ObjectPath, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
			return reply(sender, options, 0, map[string]dbus.Variant{}), nil
		},
		"Start": func(sender dbus.Sender, session dbus.ObjectPath, _ string, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
			started <- struct{}{}
			code := map[string]uint32{"cancelled": 1, "rejected": 2, "timeout": 3}[outcome]
			handle := reply(sender, options, code, map[string]dbus.Variant{})
			if code != 3 {
				if err := conn.Emit(session, sessionInterface+".Closed", map[string]dbus.Variant{}); err != nil {
					t.Error(err)
				}
			}
			return handle, nil
		},
	})
	export(desktopPath, screenCast, map[string]any{
		"SelectSources": func(sender dbus.Sender, _ dbus.ObjectPath, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
			return reply(sender, options, 0, map[string]dbus.Variant{}), nil
		},
	})

	// Open returns only after its failed request and session are retired.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	begin := time.Now()
	if _, err := Open(ctx); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("Open returned %v", err)
	}
	t.Logf("%s completed in %s", outcome, time.Since(begin))

	// No caller waits for a dialog, including when the portal never answers.
	if outcome == "missing grant" {
		if len(started) != 0 {
			t.Fatal("Start ran without the grant")
		}
		return
	}
	if len(started) != 1 || len(sessionClosed) != 1 {
		t.Fatalf("expected one attempted Start and one closed session, got %d and %d", len(started), len(sessionClosed))
	}
	if outcome == "timeout" && len(requestClosed) != 1 {
		t.Fatal("the timed-out Start request stayed open")
	}
}
