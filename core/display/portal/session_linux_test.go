package portal

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/s4wave/spacewave/core/display/gstencoder"
	"github.com/s4wave/spacewave/core/frametrack"
)

// TestInputWriteStall fills the socket of a session bus that never reads.
func TestInputWriteStall(t *testing.T) {
	// Open a private socket and keep its peer connected without draining it.
	addr := &net.UnixAddr{Name: filepath.Join(t.TempDir(), "bus"), Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	socket, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = socket.Close() }()
	peer, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = peer.Close() }()
	if err := socket.SetWriteBuffer(4096); err != nil {
		t.Fatal(err)
	}

	// Use godbus's real writer without starting authentication on the stalled bus.
	conn, err := dbus.DialUnix(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(context.Canceled)
	s := &Session{ctx: ctx, cancel: cancel, conn: conn, socket: socket, owner: desktop, path: desktopPath}

	// Force a partial write in one send and require the session to end.
	started := time.Now()
	if err := s.notify("NotifyPointerMotionAbsolute", strings.Repeat("x", 64<<10)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled write returned %v", err)
	}
	if cause := context.Cause(ctx); !errors.Is(cause, context.DeadlineExceeded) {
		t.Fatalf("stalled write left the session open: %v", cause)
	}
	t.Logf("blocked bus write ended the session in %s", time.Since(started))
}

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
	if pidFile := os.Getenv("SPACEWAVE_TEST_PLASMA_HELPER_PID"); pidFile != "" {
		measureHelperCPU(t, source, pidFile)
	}

	// Exercise the same viewer binding the admitted frame track creates.
	input := s.NewInput()
	defer func() { _ = input.Close() }()
	for _, event := range []*frametrack.Input{
		{Event: &frametrack.Input_Pointer{Pointer: &frametrack.PointerInput{X: 0.5, Y: 0.25, Buttons: 1}}},
		{Event: &frametrack.Input_Pointer{Pointer: &frametrack.PointerInput{X: 0.5, Y: 0.25}}},
	} {
		if err := input.Send(s.Context(), event); err != nil {
			t.Fatal(err)
		}
	}

	// One viewer closing must leave Shift held for the other viewer.
	shift := &frametrack.Input{Event: &frametrack.Input_Key{Key: &frametrack.KeyInput{Code: "ShiftLeft", Pressed: true}}}
	other := s.NewInput()
	for _, viewer := range []*Input{input, other} {
		if err := viewer.Send(s.Context(), shift); err != nil {
			t.Fatal(err)
		}
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	code := inputCode{code: evdevKeycodes["ShiftLeft"]}
	if got := s.pressed[code]; got != 1 {
		t.Fatalf("first viewer close left %d Shift presses, want 1", got)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if got := s.pressed[code]; got != 0 {
		t.Fatalf("last viewer close left %d Shift presses, want 0", got)
	}

	// The real portal accepts pointer events while both capabilities are live.
	notify := func(ctx context.Context) error {
		return s.conn.Object(s.owner, desktopPath).CallWithContext(ctx,
			remoteDesktop+".NotifyPointerMotion", 0, s.path, map[string]dbus.Variant{}, float64(0), float64(0)).Err
	}
	if err := notify(t.Context()); err != nil {
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
	if err := notify(t.Context()); err == nil {
		t.Fatal("portal still accepted input after the session ended")
	}
}

// measureHelperCPU samples the helper's own accounting between encoded frames.
func measureHelperCPU(t *testing.T, source *gstencoder.Source, pidFile string) {
	// Read the identity published before the wrapper execs the actual encoder.
	t.Helper()
	pid, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	stat := "/proc/" + strings.TrimSpace(string(pid)) + "/stat"
	hz, err := strconv.ParseFloat(os.Getenv("SPACEWAVE_TEST_PLASMA_CLK_TCK"), 64)
	if err != nil || hz <= 0 {
		t.Fatalf("invalid CLK_TCK: %v", err)
	}

	// Measure steady state with an animated private output to keep it producing.
	const frames = 120
	start := time.Now()
	before := readCPUTicks(t, stat)
	for range frames {
		if _, _, err := source.ReadFrame(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	ms := float64(readCPUTicks(t, stat)-before) * 1000 / hz / frames
	t.Logf("encoder CPU on %d frames: %.3f ms/frame at %.2f fps", frames, ms, frames/time.Since(start).Seconds())
}

// readCPUTicks returns user and system ticks from a Linux proc stat entry.
func readCPUTicks(t *testing.T, path string) int64 {
	// Fields after the command name start at field 3, even when the name has spaces.
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	fields := strings.Fields(text[strings.LastIndex(text, ")")+1:])

	// Include CPU from GStreamer's threads in the encoder process.
	var ticks int64
	for _, field := range fields[11:13] {
		value, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		ticks += value
	}
	return ticks
}
