//go:build !js && !windows

// Package nativeendpoint serves the private SRPC endpoints inherited by a
// native viewer process.
package nativeendpoint

import (
	"context"
	"errors"
	"unicode"
	"unicode/utf8"

	"github.com/aperturerobotics/starpc/srpc"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/bldr/tui/nativehost"
	command_registry "github.com/s4wave/spacewave/sdk/command/registry"
	native "github.com/s4wave/spacewave/sdk/viewer/native"
)

// Config describes the selected Console resource and its selected state.
type Config struct {
	// ResourceClient is the SRPC client for the selected Console resource.
	ResourceClient srpc.Client
	// StateStore stores the selected viewer state.
	StateStore resource_state.StateAtomStore
	// SelectedStateKey identifies the state atom served by StateService.
	SelectedStateKey string
	// CommandRegistryClient serves the daemon command registry.
	CommandRegistryClient command_registry.SRPCCommandRegistryResourceServiceClient
}

// Factory creates one independent set of native viewer endpoints per Open.
type Factory struct {
	// config is the validated immutable endpoint configuration.
	config Config
}

// New constructs an endpoint factory after validating its immutable config.
func New(c Config) (*Factory, error) {
	if err := validateConfig(c); err != nil {
		return nil, err
	}
	return &Factory{config: c}, nil
}

// NewEndpointFactory returns a nativehost endpoint factory.
func NewEndpointFactory(c Config) (nativehost.EndpointFactory, error) {
	f, err := New(c)
	if err != nil {
		return nil, err
	}
	return f.Open, nil
}

// Open creates fresh endpoint descriptors for one child attempt.
func (f *Factory) Open(ctx context.Context) (*nativehost.EndpointSet, error) {
	if f == nil {
		return nil, errors.New("native endpoint factory is nil")
	}
	if ctx == nil {
		return nil, errors.New("native endpoint context is nil")
	}
	return open(ctx, f.config)
}

// validateConfig requires every endpoint dependency and a bounded selected state identity.
func validateConfig(c Config) error {
	if c.ResourceClient == nil {
		return errors.New("selected resource client is required")
	}
	if c.StateStore == nil {
		return errors.New("state store is required")
	}
	if c.CommandRegistryClient == nil {
		return errors.New("command registry client is required")
	}
	if !validIdentity(c.SelectedStateKey) {
		return errors.New("selected state key is invalid")
	}
	return nil
}

// validIdentity accepts bounded UTF-8 identities without control characters.
func validIdentity(value string) bool {
	if value == "" || len(value) > native.MaxIdentityBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
