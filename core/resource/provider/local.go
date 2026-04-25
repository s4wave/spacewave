package resource_provider

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	"github.com/sirupsen/logrus"
)

// LocalProviderResource implements the LocalProviderResourceService.
type LocalProviderResource struct {
	*ProviderResource
	le       *logrus.Entry
	b        bus.Bus
	provider *provider_local.Provider
}

// NewLocalProviderResource creates a new LocalProviderResource.
func NewLocalProviderResource(pr *ProviderResource, le *logrus.Entry, b bus.Bus, prov *provider_local.Provider) *LocalProviderResource {
	return &LocalProviderResource{
		ProviderResource: pr,
		le:               le,
		b:                b,
		provider:         prov,
	}
}

// CreateAccount creates a ProviderAccount and Session on the local provider.
func (s *LocalProviderResource) CreateAccount(
	ctx context.Context,
	req *s4wave_provider_local.CreateAccountRequest,
) (*s4wave_provider_local.CreateAccountResponse, error) {
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	sessRef, err := s.provider.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		return nil, err
	}

	meta := &session.SessionMetadata{
		ProviderDisplayName: "Local",
		ProviderId:          "local",
		ProviderAccountId:   sessRef.GetProviderResourceRef().GetProviderAccountId(),
		CreatedAt:           time.Now().UnixMilli(),
	}
	listEntry, err := sessionCtrl.RegisterSession(ctx, sessRef, meta)
	if err != nil {
		return nil, err
	}

	return &s4wave_provider_local.CreateAccountResponse{SessionListEntry: listEntry}, nil
}

// _ is a type assertion
var _ s4wave_provider_local.SRPCLocalProviderResourceServiceServer = ((*LocalProviderResource)(nil))
