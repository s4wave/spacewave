package s4wave_root

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/core/changelog"
	"github.com/s4wave/spacewave/core/provider"
	session "github.com/s4wave/spacewave/core/session"
	hash "github.com/s4wave/spacewave/net/hash"
)

// Root is the top-level entrypoint for accessing resources on the SDK.
// Root allows accessing all other resources from the top level.
//
// This Go SDK implementation wraps RootResourceService.
type Root struct {
	client  *resource_client.Client
	ref     resource_client.ResourceRef
	service SRPCRootResourceServiceClient
}

// NewRoot creates a new Root resource wrapper.
func NewRoot(client *resource_client.Client, ref resource_client.ResourceRef) (*Root, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &Root{
		client:  client,
		ref:     ref,
		service: NewSRPCRootResourceServiceClient(srpcClient),
	}, nil
}

// GetResourceRef returns the resource reference.
func (r *Root) GetResourceRef() resource_client.ResourceRef {
	return r.ref
}

// Release releases the resource reference.
func (r *Root) Release() {
	r.ref.Release()
}

// LookupProvider accesses a provider Resource by ID.
// Returns the resource ID for the provider.
func (r *Root) LookupProvider(ctx context.Context, providerID string) (uint32, error) {
	resp, err := r.service.LookupProvider(ctx, &LookupProviderRequest{ProviderId: providerID})
	if err != nil {
		return 0, err
	}
	return resp.GetResourceId(), nil
}

// MountSession mounts a session and returns the resource ID for the Session resource.
// The MountSession directive will remain active until the returned Resource is released.
func (r *Root) MountSession(ctx context.Context, ref *session.SessionRef) (uint32, error) {
	resp, err := r.service.MountSession(ctx, &MountSessionRequest{SessionRef: ref})
	if err != nil {
		return 0, err
	}
	return resp.GetResourceId(), nil
}

// MountSessionByIdx mounts a session by index and returns the resource ID and session ref.
// Returns the resource ID, session ref, and whether the session was not found.
func (r *Root) MountSessionByIdx(ctx context.Context, idx uint32) (*MountSessionByIdxResponse, error) {
	return r.service.MountSessionByIdx(ctx, &MountSessionByIdxRequest{SessionIdx: idx})
}

// AccessStateAtom accesses the global state atom resource.
// Returns the resource ID for the StateAtom which provides Get/Set/Watch RPCs.
func (r *Root) AccessStateAtom(ctx context.Context, storeID string) (uint32, error) {
	resp, err := r.service.AccessStateAtom(ctx, &AccessStateAtomRequest{StoreId: storeID})
	if err != nil {
		return 0, err
	}
	return resp.GetResourceId(), nil
}

// AccessWebListener creates or reuses a localhost web listener.
func (r *Root) AccessWebListener(
	ctx context.Context,
	listenMultiaddr string,
	background bool,
) (*AccessWebListenerResponse, error) {
	return r.service.AccessWebListener(ctx, &AccessWebListenerRequest{
		ListenMultiaddr: listenMultiaddr,
		Background:      background,
	})
}

// ListWebListeners lists daemon-owned localhost web listeners.
func (r *Root) ListWebListeners(ctx context.Context) ([]*WebListenerInfo, error) {
	strm, err := r.service.WatchWebListeners(ctx, &WatchWebListenersRequest{})
	if err != nil {
		return nil, err
	}
	defer strm.Close()
	resp, err := strm.Recv()
	if err != nil {
		return nil, err
	}
	return resp.GetListeners(), nil
}

// StopWebListener stops a daemon-owned localhost web listener.
func (r *Root) StopWebListener(ctx context.Context, listenerID string) (bool, error) {
	resp, err := r.service.StopWebListener(ctx, &StopWebListenerRequest{ListenerId: listenerID})
	if err != nil {
		return false, err
	}
	return !resp.GetNotFound(), nil
}

// MarshalHash marshals a Hash to a base58 string.
func (r *Root) MarshalHash(ctx context.Context, h *hash.Hash) (string, error) {
	resp, err := r.service.MarshalHash(ctx, &MarshalHashRequest{Hash: h})
	if err != nil {
		return "", err
	}
	return resp.GetHashStr(), nil
}

// ParseHash parses a Hash from a base58 string.
func (r *Root) ParseHash(ctx context.Context, hashStr string) (*hash.Hash, error) {
	resp, err := r.service.ParseHash(ctx, &ParseHashRequest{HashStr: hashStr})
	if err != nil {
		return nil, err
	}
	return resp.GetHash(), nil
}

// HashSum computes a hash of the given data with the specified hash type.
func (r *Root) HashSum(ctx context.Context, hashType hash.HashType, data []byte) (*hash.Hash, error) {
	resp, err := r.service.HashSum(ctx, &HashSumRequest{HashType: hashType, Data: data})
	if err != nil {
		return nil, err
	}
	return resp.GetHash(), nil
}

// HashValidate validates a hash object.
// Returns whether the hash is valid and any validation error message.
func (r *Root) HashValidate(ctx context.Context, h *hash.Hash) (bool, string, error) {
	resp, err := r.service.HashValidate(ctx, &HashValidateRequest{Hash: h})
	if err != nil {
		return false, "", err
	}
	return resp.GetValid(), resp.GetError(), nil
}

// ListProviders lists the available providers.
func (r *Root) ListProviders(ctx context.Context) ([]*provider.ProviderInfo, error) {
	resp, err := r.service.ListProviders(ctx, &ListProvidersRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetProviders(), nil
}

// ListSessions lists the configured sessions.
func (r *Root) ListSessions(ctx context.Context) ([]*session.SessionListEntry, error) {
	resp, err := r.service.ListSessions(ctx, &ListSessionsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetSessions(), nil
}

// DeleteSession removes a session from the local session list by index.
func (r *Root) DeleteSession(ctx context.Context, sessionIdx uint32) error {
	_, err := r.service.DeleteSession(ctx, &DeleteSessionRequest{
		SessionIdx: sessionIdx,
	})
	return err
}

// GetChangelog returns the application changelog.
func (r *Root) GetChangelog(ctx context.Context) (*changelog.Changelog, error) {
	resp, err := r.service.GetChangelog(ctx, &GetChangelogRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetChangelog(), nil
}

// GetCdn accesses a configured CDN resource by id.
func (r *Root) GetCdn(ctx context.Context, cdnID string) (*GetCdnResponse, error) {
	return r.service.GetCdn(ctx, &GetCdnRequest{CdnId: cdnID})
}

// UnlockSession unlocks a PIN-locked session before mounting.
func (r *Root) UnlockSession(ctx context.Context, sessionIdx uint32, pin []byte) error {
	_, err := r.service.UnlockSession(ctx, &UnlockSessionByIdxRequest{
		SessionIdx: sessionIdx,
		Pin:        pin,
	})
	return err
}
