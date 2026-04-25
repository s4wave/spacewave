//go:build js

package resource_account

import (
	"context"

	"github.com/pkg/errors"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// StartDesktopPasskeyRegisterHandoff is unavailable in the browser runtime.
func (r *AccountResource) StartDesktopPasskeyRegisterHandoff(
	_ context.Context,
	_ *s4wave_account.StartDesktopPasskeyRegisterHandoffRequest,
) (*s4wave_account.StartDesktopPasskeyRegisterHandoffResponse, error) {
	return nil, errors.New("desktop passkey register handoff is only available in native builds")
}
