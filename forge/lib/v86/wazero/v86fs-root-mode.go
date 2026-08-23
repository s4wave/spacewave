package v86_wazero

import (
	"strings"

	"github.com/pkg/errors"
)

const (
	rootModeReadonly = "readonly"
	rootModeRAM      = "ram"
	rootModeDisk     = "disk"
	rootModeVolume   = "volume"
	rootModeDaemon   = "daemon"

	rootModeAllowed = "readonly|ram|disk=<path>|volume=<file>|daemon=<space>"
)

// RootMode describes the backing store used for the guest root overlay.
type RootMode struct {
	Mode string
	Arg  string
}

// ParseRootMode parses a guest root backing mode string.
func ParseRootMode(value string) (RootMode, error) {
	if value == "" {
		value = rootModeRAM
	}
	mode, arg, hasArg := strings.Cut(value, "=")
	switch mode {
	case rootModeReadonly, rootModeRAM:
		if hasArg {
			return RootMode{}, errors.Errorf("root-mode %s does not take an argument", mode)
		}
		return RootMode{Mode: mode}, nil
	case rootModeDisk, rootModeVolume, rootModeDaemon:
		if !hasArg || arg == "" {
			return RootMode{}, errors.Errorf("root-mode %s requires an argument, want %s", mode, rootModeAllowed)
		}
		return RootMode{Mode: mode, Arg: arg}, nil
	default:
		return RootMode{}, errors.Errorf("unknown root-mode %q, want %s", mode, rootModeAllowed)
	}
}

// normalizeRootMode applies the empty-mode default and validates the mode
func normalizeRootMode(mode RootMode) (RootMode, error) {
	if mode.Mode == "" {
		mode.Mode = rootModeRAM
	}
	switch mode.Mode {
	case rootModeReadonly, rootModeRAM:
		if mode.Arg != "" {
			return RootMode{}, errors.Errorf("root-mode %s does not take an argument", mode.Mode)
		}
		return mode, nil
	case rootModeDisk, rootModeVolume, rootModeDaemon:
		if mode.Arg == "" {
			return RootMode{}, errors.Errorf("root-mode %s requires an argument, want %s", mode.Mode, rootModeAllowed)
		}
		return mode, nil
	default:
		return RootMode{}, errors.Errorf("unknown root-mode %q, want %s", mode.Mode, rootModeAllowed)
	}
}
