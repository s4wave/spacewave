package s4wave_terminal

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/protocol"
)

// TerminalTypeID is the type identifier for remote Terminal objects.
const TerminalTypeID = "spacewave/terminal"

// RemoteShellProtocolID is the Bifrost protocol used for PTY-backed terminals.
const RemoteShellProtocolID = protocol.ID("alpha/remote-shell/v0")

const (
	// DefaultTerminalCols is the fallback terminal width.
	DefaultTerminalCols uint32 = 80
	// DefaultTerminalRows is the fallback terminal height.
	DefaultTerminalRows uint32 = 24
)

// NewTerminalBlock constructs a new Terminal block.
func NewTerminalBlock() block.Block {
	return &Terminal{}
}

// UnmarshalTerminal unmarshals a Terminal from a cursor.
func UnmarshalTerminal(ctx context.Context, bcs *block.Cursor) (*Terminal, error) {
	return block.UnmarshalBlock[*Terminal](ctx, bcs, NewTerminalBlock)
}

// MarshalBlock marshals the Terminal to bytes.
func (t *Terminal) MarshalBlock() ([]byte, error) {
	return t.MarshalVT()
}

// UnmarshalBlock unmarshals the Terminal from bytes.
func (t *Terminal) UnmarshalBlock(data []byte) error {
	return t.UnmarshalVT(data)
}

// Validate performs cursory checks on the Terminal block.
func (t *Terminal) Validate() error {
	if strings.TrimSpace(t.GetName()) == "" {
		return errors.New("terminal name is required")
	}
	if strings.TrimSpace(t.GetDeviceObjectKey()) == "" {
		return errors.New("terminal device object key is required")
	}
	if strings.TrimSpace(t.GetDevicePeerId()) == "" {
		return errors.New("terminal device peer id is required")
	}
	return validateTerminalEnvironment(t.GetEnvironment())
}

func validateTerminalEnvironment(env []string) error {
	for _, entry := range env {
		if strings.TrimSpace(entry) == "" {
			return errors.New("terminal environment entry is empty")
		}
		key, _, found := strings.Cut(entry, "=")
		if !found || strings.TrimSpace(key) == "" {
			return errors.Errorf("terminal environment entry %q must be KEY=VALUE", entry)
		}
	}
	return nil
}

// NormalizeTerminalFrameSize fills missing terminal dimensions.
func NormalizeTerminalFrameSize(cols, rows uint32) (uint32, uint32) {
	if cols == 0 {
		cols = DefaultTerminalCols
	}
	if rows == 0 {
		rows = DefaultTerminalRows
	}
	return cols, rows
}

// _ is a type assertion
var _ block.Block = (*Terminal)(nil)
