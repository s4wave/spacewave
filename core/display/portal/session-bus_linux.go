package portal

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"
)

// dialSessionBus opens the local socket so capture can interrupt transport writes.
func dialSessionBus(ctx context.Context) (*net.UnixConn, error) {
	// Use the desktop user's socket when its default address is implicit.
	addresses := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if addresses == "" || addresses == "autolaunch:" {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			runtimeDir = filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
		}
		addresses = "unix:path=" + dbus.EscapeBusAddressValue(filepath.Join(runtimeDir, "bus"))
	}

	// Follow D-Bus address order and preserve Linux abstract socket names.
	var err error
	var dialer net.Dialer
	for address := range strings.SplitSeq(addresses, ";") {
		if name := unixBusName(address); name != "" {
			var socket net.Conn
			socket, err = dialer.DialContext(ctx, "unix", name)
			if err == nil {
				return socket.(*net.UnixConn), nil
			}
		}
	}
	if err == nil {
		err = errors.New("display capture requires a local Unix session bus")
	}
	return nil, err
}

// unixBusName extracts a filesystem or abstract socket from a D-Bus address.
func unixBusName(address string) string {
	// A local Unix transport is required by the portal's PipeWire FD reply.
	fields, ok := strings.CutPrefix(address, "unix:")
	if !ok {
		return ""
	}

	// Ignore metadata such as guid, which does not identify the socket.
	for field := range strings.SplitSeq(fields, ",") {
		key, value, _ := strings.Cut(field, "=")
		name, err := dbus.UnescapeBusAddressValue(value)
		if err != nil || name == "" {
			continue
		}
		switch key {
		case "path":
			return name
		case "abstract":
			return "@" + name
		}
	}
	return ""
}
