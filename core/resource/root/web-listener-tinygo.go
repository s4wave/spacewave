//go:build tinygo

package resource_root

import (
	"context"

	"github.com/pkg/errors"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"github.com/sirupsen/logrus"
)

type webListenerRegistry struct{}

func newWebListenerRegistry(_ *logrus.Entry) *webListenerRegistry {
	return &webListenerRegistry{}
}

func (r *webListenerRegistry) close() {}

// AccessWebListener returns unsupported in TinyGo browser builds.
func (s *CoreRootServer) AccessWebListener(
	_ context.Context,
	_ *s4wave_root.AccessWebListenerRequest,
) (*s4wave_root.AccessWebListenerResponse, error) {
	return nil, errors.New("web listener is not supported in TinyGo browser builds")
}

// WatchWebListeners returns unsupported in TinyGo browser builds.
func (s *CoreRootServer) WatchWebListeners(
	_ *s4wave_root.WatchWebListenersRequest,
	_ s4wave_root.SRPCRootResourceService_WatchWebListenersStream,
) error {
	return errors.New("web listener is not supported in TinyGo browser builds")
}

// StopWebListener returns unsupported in TinyGo browser builds.
func (s *CoreRootServer) StopWebListener(
	_ context.Context,
	_ *s4wave_root.StopWebListenerRequest,
) (*s4wave_root.StopWebListenerResponse, error) {
	return nil, errors.New("web listener is not supported in TinyGo browser builds")
}
