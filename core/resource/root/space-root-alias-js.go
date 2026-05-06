//go:build js

package resource_root

import (
	"context"

	"github.com/pkg/errors"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// ListSpaceRootAliases is not available in browser runtimes.
func (s *CoreRootServer) ListSpaceRootAliases(
	context.Context,
	*s4wave_root.ListSpaceRootAliasesRequest,
) (*s4wave_root.ListSpaceRootAliasesResponse, error) {
	return nil, errors.New("space root alias registry is only available in native runtimes")
}

// WatchSpaceRootAliases is not available in browser runtimes.
func (s *CoreRootServer) WatchSpaceRootAliases(
	_ *s4wave_root.WatchSpaceRootAliasesRequest,
	_ s4wave_root.SRPCRootResourceService_WatchSpaceRootAliasesStream,
) error {
	return errors.New("space root alias registry is only available in native runtimes")
}

// UpsertSpaceRootAlias is not available in browser runtimes.
func (s *CoreRootServer) UpsertSpaceRootAlias(
	context.Context,
	*s4wave_root.UpsertSpaceRootAliasRequest,
) (*s4wave_root.UpsertSpaceRootAliasResponse, error) {
	return nil, errors.New("space root alias registry is only available in native runtimes")
}

// RemoveSpaceRootAlias is not available in browser runtimes.
func (s *CoreRootServer) RemoveSpaceRootAlias(
	context.Context,
	*s4wave_root.RemoveSpaceRootAliasRequest,
) (*s4wave_root.RemoveSpaceRootAliasResponse, error) {
	return nil, errors.New("space root alias registry is only available in native runtimes")
}

// WatchSpaceRootRuntime is not available in browser runtimes.
func (s *CoreRootServer) WatchSpaceRootRuntime(
	_ *s4wave_root.WatchSpaceRootRuntimeRequest,
	_ s4wave_root.SRPCRootResourceService_WatchSpaceRootRuntimeStream,
) error {
	return errors.New("space root runtime loading is only available in native runtimes")
}
