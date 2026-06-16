package electron

// effectiveDesktopPresencePolicy returns the presence policy the Electron runtime must use on the given GOOS.
// macOS hosts the native menu bar, which owns background presence, so Electron always uses window-lifetime there
// regardless of the configured policy; other platforms use the configured value.
func effectiveDesktopPresencePolicy(configured DesktopPresencePolicy, goos string) DesktopPresencePolicy {
	if goos == "darwin" {
		return DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME
	}
	return configured
}
