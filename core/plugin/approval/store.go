// Package plugin_approval keeps each device person's decisions about plugins
// from outside the release, and admits the manifest roots they allowed.
//
// The decisions live in the device-local platform-account ObjectStore, keyed
// by the session peer that made them. Nothing in a shared Space is read, so a
// member who writes an approval-shaped object into a Space changes nothing.
package plugin_approval

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
)

// KeyPrefix is the prefix of plugin approval keys.
const KeyPrefix = "plugin-approval"

// Key returns the KV key of one decision.
// Key format: plugin-approval/{peerID}/{pluginID}/{manifestRoot}
func Key(peerID, pluginID, manifestRoot string) []byte {
	return []byte(strings.Join([]string{KeyPrefix, peerID, pluginID, manifestRoot}, "/"))
}

// GetApproval reads one decision. It returns nil, nil when there is none.
func GetApproval(ctx context.Context, store kvtx.Store, peerID, pluginID, manifestRoot string) (*s4wave_plugin_approval.PluginApproval, error) {
	key := Key(peerID, pluginID, manifestRoot)
	var out *s4wave_plugin_approval.PluginApproval
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Read the stored decision and decode it when present.
			data, found, err := tx.Get(ctx, key)
			if err != nil || !found {
				return err
			}
			next := &s4wave_plugin_approval.PluginApproval{}
			if err := next.UnmarshalVT(data); err != nil {
				return errors.Wrap(err, "decode plugin approval")
			}
			out = next
			return nil
		},
	)
	return out, err
}

// PutApproval writes one decision under its peer, plugin and root.
func PutApproval(ctx context.Context, store kvtx.Store, peerID string, approval *s4wave_plugin_approval.PluginApproval) error {
	data, err := approval.MarshalVT()
	if err != nil {
		return err
	}
	key := Key(peerID, approval.GetPluginId(), approval.GetManifestRoot())
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, key, data)
		},
	)
}

// ScanApprovals calls cb with each of peerID's decisions in key order. A
// record that does not decode stops the scan with its error.
func ScanApprovals(ctx context.Context, store kvtx.Store, peerID string, cb func(*s4wave_plugin_approval.PluginApproval) error) error {
	// Open a read transaction.
	prefix := []byte(KeyPrefix + "/" + peerID + "/")
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Decode each of peerID's records in key order.
	return tx.ScanPrefix(ctx, prefix, func(_, value []byte) error {
		approval := &s4wave_plugin_approval.PluginApproval{}
		if err := approval.UnmarshalVT(value); err != nil {
			return errors.Wrap(err, "decode plugin approval")
		}
		return cb(approval)
	})
}
