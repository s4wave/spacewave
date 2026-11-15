package bldr_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
)

// AdmitSource names why a plugin manifest root is admitted.
type AdmitSource uint8

const (
	// AdmitSourceTrusted admits a root the host trusts: one its signed release
	// or its development build supplied.
	AdmitSourceTrusted AdmitSource = iota + 1
	// AdmitSourceApproved admits a root the device's person allowed.
	AdmitSourceApproved
)

// String returns the source's name.
func (s AdmitSource) String() string {
	switch s {
	case AdmitSourceTrusted:
		return "trusted"
	case AdmitSourceApproved:
		return "approved"
	default:
		return "unknown"
	}
}

// AdmitPluginRoot asks whether one plugin manifest root may run for one peer.
//
// A resolver adds a value while it admits the root and removes it when it
// stops. A root with no value is not admitted, so a bus with no resolver
// admits nothing: admission fails closed.
type AdmitPluginRoot interface {
	// Directive indicates AdmitPluginRoot is a directive.
	directive.Directive

	// AdmitPluginRootPeerID returns the b58 peer ID of the scheduler asking,
	// which names the session the root would run for.
	AdmitPluginRootPeerID() string
	// AdmitPluginRootPluginID returns the plugin ID.
	AdmitPluginRootPluginID() string
	// AdmitPluginRootManifestRoot returns the b58 manifest root block hash.
	AdmitPluginRootManifestRoot() string
}

// AdmitPluginRootValue is the result type for AdmitPluginRoot.
type AdmitPluginRootValue = AdmitSource

// admitPluginRoot implements AdmitPluginRoot.
type admitPluginRoot struct {
	peerID, pluginID, manifestRoot string
}

// NewAdmitPluginRoot constructs an AdmitPluginRoot directive.
func NewAdmitPluginRoot(peerID, pluginID, manifestRoot string) AdmitPluginRoot {
	return &admitPluginRoot{peerID: peerID, pluginID: pluginID, manifestRoot: manifestRoot}
}

// WaitPluginRootAdmitted blocks until a resolver on b admits the root, then
// releases the directive. It returns only with ctx's error otherwise.
func WaitPluginRootAdmitted(ctx context.Context, b bus.Bus, peerID, pluginID, manifestRoot string) (AdmitSource, error) {
	val, _, ref, err := bus.ExecWaitValue[AdmitPluginRootValue](
		ctx,
		b,
		NewAdmitPluginRoot(peerID, pluginID, manifestRoot),
		nil,
		nil,
		nil,
	)
	if err != nil {
		return 0, err
	}
	ref.Release()
	return val, nil
}

// AdmitPluginRootPeerID returns the peer ID.
func (d *admitPluginRoot) AdmitPluginRootPeerID() string {
	return d.peerID
}

// AdmitPluginRootPluginID returns the plugin ID.
func (d *admitPluginRoot) AdmitPluginRootPluginID() string {
	return d.pluginID
}

// AdmitPluginRootManifestRoot returns the manifest root.
func (d *admitPluginRoot) AdmitPluginRootManifestRoot() string {
	return d.manifestRoot
}

// Validate validates the directive.
func (d *admitPluginRoot) Validate() error {
	if d.pluginID == "" {
		return ErrEmptyPluginID
	}
	if d.manifestRoot == "" {
		return errors.New("manifest root cannot be empty")
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *admitPluginRoot) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent reports whether other asks about the same peer, plugin and root.
func (d *admitPluginRoot) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(AdmitPluginRoot)
	return ok &&
		od.AdmitPluginRootPeerID() == d.peerID &&
		od.AdmitPluginRootPluginID() == d.pluginID &&
		od.AdmitPluginRootManifestRoot() == d.manifestRoot
}

// GetName returns the directive's type name.
func (d *admitPluginRoot) GetName() string {
	return "AdmitPluginRoot"
}

// GetDebugVals returns the directive arguments stringified.
func (d *admitPluginRoot) GetDebugVals() directive.DebugValues {
	// List the peer, plugin and root the directive asks about.
	vals := directive.DebugValues{}
	vals["peer-id"] = []string{d.peerID}
	vals["plugin-id"] = []string{d.pluginID}
	vals["manifest-root"] = []string{d.manifestRoot}
	return vals
}

// _ is a type assertion
var (
	_ AdmitPluginRoot              = (*admitPluginRoot)(nil)
	_ directive.DirectiveWithEquiv = (*admitPluginRoot)(nil)
)
