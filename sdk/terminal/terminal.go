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
	switch EffectiveTerminalTargetKind(t) {
	case TerminalTargetKind_TERMINAL_TARGET_KIND_DEVICE:
		if strings.TrimSpace(t.GetDeviceObjectKey()) == "" {
			return errors.New("terminal device object key is required")
		}
		if strings.TrimSpace(t.GetDevicePeerId()) == "" {
			return errors.New("terminal device peer id is required")
		}
	case TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST:
		if strings.TrimSpace(t.GetSshHostObjectKey()) == "" {
			return errors.New("terminal SSH Host object key is required")
		}
	default:
		return errors.New("terminal target is required")
	}
	return validateTerminalEnvironment(t.GetEnvironment())
}

// terminalTargetSource is the field set that identifies a terminal target.
// Both Terminal and CreateTerminalOp expose these getters.
type terminalTargetSource interface {
	GetTargetKind() TerminalTargetKind
	GetSshHostObjectKey() string
	GetDeviceObjectKey() string
	GetDevicePeerId() string
}

// resolveTerminalTargetKind resolves targets with no explicit kind by their
// set fields: an SSH host key wins, then device key or peer ID.
func resolveTerminalTargetKind(t terminalTargetSource) TerminalTargetKind {
	if t == nil {
		return TerminalTargetKind_TERMINAL_TARGET_KIND_UNKNOWN
	}
	switch t.GetTargetKind() {
	case TerminalTargetKind_TERMINAL_TARGET_KIND_DEVICE,
		TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST:
		return t.GetTargetKind()
	}
	if strings.TrimSpace(t.GetSshHostObjectKey()) != "" {
		return TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST
	}
	if strings.TrimSpace(t.GetDeviceObjectKey()) != "" || strings.TrimSpace(t.GetDevicePeerId()) != "" {
		return TerminalTargetKind_TERMINAL_TARGET_KIND_DEVICE
	}
	return TerminalTargetKind_TERMINAL_TARGET_KIND_UNKNOWN
}

// EffectiveTerminalTargetKind resolves old Device terminals with no explicit target kind.
func EffectiveTerminalTargetKind(t *Terminal) TerminalTargetKind {
	return resolveTerminalTargetKind(t)
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

var _ block.Block = (*Terminal)(nil)
