//go:build !js

package bldr_tui_host

import "io"

// Config configures one generic Bun TuiView host.
type Config struct {
	BunPath          string
	ModuleURL        string
	ExportName       string
	PluginID         string
	DaemonSocketPath string
	SessionIndex     uint32
	SessionObjectKey string
	SpaceName        string
	StateStoreID     string
	RestartLimit     uint
	Stdin            io.Reader
	Stdout           io.Writer
	Stderr           io.Writer
}
