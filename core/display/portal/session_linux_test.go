package portal

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/s4wave/spacewave/core/display/gstencoder"
	"github.com/s4wave/spacewave/core/frametrack"
)

// TestPortalCapture reads hardware AVC and ends both capabilities on portal events.
func TestPortalCapture(t *testing.T) {
	// The fixture owns a separate D-Bus, compositor and PermissionStore.
	helper := os.Getenv("SPACEWAVE_TEST_PRIVATE_PLASMA_HELPER")
	if helper == "" {
		t.Skip("run against the private Plasma integration fixture")
	}

	// A missing grant refuses capture before Start.
	_, err := Open(t.Context())
	if !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("unprovisioned display returned %v", err)
	}

	// Provision once so the second session has to reuse the persisted grant.
	grant := os.Getenv("SPACEWAVE_TEST_PRIVATE_PLASMA_PROVISION")
	grantOutput, err := exec.CommandContext(t.Context(), grant).CombinedOutput()
	if err != nil {
		t.Fatalf("provision capture: %v, %s", err, grantOutput)
	}

	// Drive each loss condition from the real portal service that owns it.
	t.Run("SessionClosed", func(t *testing.T) {
		checkPortalCapture(t, helper, ErrSessionClosed, func(s *Session) error {
			return s.conn.Object("org.freedesktop.impl.portal.desktop.kde", s.path).
				CallWithContext(t.Context(), "org.freedesktop.impl.portal.Session.Close", 0).Err
		})
	})
	t.Run("GrantRevoked", func(t *testing.T) {
		checkPortalCapture(t, helper, ErrNotAuthorized, func(s *Session) error {
			return s.conn.Object(permissions, permissionPath).CallWithContext(t.Context(),
				permissions+".DeletePermission", 0, permissionTable, permissionID, AppID).Err
		})
	})
}

// checkPortalCapture starts one connection, reads its first frame and ends it.
func checkPortalCapture(t *testing.T, helper string, stopped error, end func(*Session) error) {
	// Open the session using the grant installed by the provisioning script.
	t.Helper()
	s, err := Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Give the real encoder only the portal's descriptor and selected node.
	remote, nodeID, err := s.OpenPipeWireRemote()
	if err != nil {
		t.Fatal(err)
	}
	source, err := gstencoder.OpenPipeWireSource(s.Context(), helper, remote, nodeID)
	_ = remote.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })

	// Verify the picture reaches the same Source used by the Display track.
	header, frame, err := source.ReadFrame(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if header.GetWidth() != 1920 || header.GetHeight() != 1080 ||
		header.GetCodec() != frametrack.Codec_CODEC_H264_AVC ||
		len(header.GetDecoderDescription()) == 0 || !frame.GetKeyframe() || len(frame.GetData()) == 0 {
		t.Fatalf("expected 1920x1080 AVC IDR, got header %v and %d bytes", header, len(frame.GetData()))
	}
	t.Logf("Start to first picture: %s, node %d, %d bytes", time.Since(s.startedAt), nodeID, len(frame.GetData()))

	// The real portal accepts pointer events while both capabilities are live.
	input := func(ctx context.Context) error {
		return s.conn.Object(s.owner, desktopPath).CallWithContext(ctx,
			remoteDesktop+".NotifyPointerMotion", 0, s.path, map[string]dbus.Variant{}, float64(0), float64(0)).Err
	}
	if err := input(t.Context()); err != nil {
		t.Fatalf("portal rejected input on live session: %v", err)
	}

	// Verify the reason reaches the frame reader and the portal closes input.
	if err := end(s); err != nil {
		t.Fatal(err)
	}
	<-s.watched
	if _, _, err := source.ReadFrame(t.Context()); !errors.Is(err, stopped) {
		t.Fatalf("stopped source returned %v, want %v", err, stopped)
	}
	if err := input(t.Context()); err == nil {
		t.Fatal("portal still accepted input after the session ended")
	}
}
