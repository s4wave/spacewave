//go:build js

package s4wave_terminal

import (
	"context"

	"github.com/pkg/errors"
)

func (r *TerminalResource) connectSshHostTerminal(
	ctx context.Context,
	cancel context.CancelFunc,
	strm SRPCTerminalResourceService_ConnectTerminalStream,
	current *Terminal,
) error {
	return errors.New("SSH Host terminals require a native runtime")
}
