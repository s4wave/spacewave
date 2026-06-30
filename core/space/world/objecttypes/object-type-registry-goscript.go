//go:build goscript

package objecttypes

import (
	"context"

	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_world "github.com/s4wave/spacewave/sdk/chat/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_device_world "github.com/s4wave/spacewave/sdk/device/world"
	s4wave_git_world "github.com/s4wave/spacewave/sdk/git/world"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_sshhost_world "github.com/s4wave/spacewave/sdk/sshhost/world"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_terminal_world "github.com/s4wave/spacewave/sdk/terminal/world"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// LookupObjectType looks up a GoScript-supported object type by ID.
// Returns nil if not found.
func LookupObjectType(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
	switch typeID {
	case s4wave_layout_world.ObjectLayoutTypeID:
		return s4wave_layout_world.ObjectLayoutType, nil
	case s4wave_unixfs_world.UnixFSTypeID:
		return s4wave_unixfs_world.UnixFSType, nil
	case s4wave_git_world.GitRepoTypeID:
		return s4wave_git_world.GitRepoType, nil
	case s4wave_canvas_world.CanvasTypeID:
		return s4wave_canvas_world.CanvasType, nil
	case s4wave_git_world.GitWorktreeTypeID:
		return s4wave_git_world.GitWorktreeType, nil
	case s4wave_kv_world.KvStoreTypeID:
		return s4wave_kv_world.KvStoreType, nil
	// TODO: move SQL ObjectTypes into a separate spacewave-sql plugin.
	// SQL stays out of spacewave-core so go-mysql-server is not in the core bundle.
	// When wiring that plugin, pass the sql_lite build tag so go-mysql-server
	// omits heavy features like collation maps.
	case spacewave_chat.ChatChannelTypeID:
		return spacewave_chat_world.ChatChannelType, nil
	case spacewave_chat.ChatMessageTypeID:
		return spacewave_chat_world.ChatMessageType, nil
	case s4wave_device.ComputersDashboardTypeID:
		return s4wave_device_world.ComputersDashboardType, nil
	case s4wave_terminal.TerminalTypeID:
		return s4wave_terminal_world.TerminalType, nil
	case s4wave_sshhost.SshHostTypeID:
		return s4wave_sshhost_world.SshHostType, nil
	default:
		return s4wave_wizard.LookupWizardObjectType(ctx, typeID)
	}
}
