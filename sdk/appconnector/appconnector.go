// Package s4wave_appconnector holds the AppConnector and AppSnapshot objects.
// An AppConnector is a Space's authorized connection to one application's
// admin API. The fetcher of the device that approved its ProcessBinding keeps
// the connector's AppSnapshot current.
package s4wave_appconnector

import (
	"context"
	"net/url"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

const (
	// AppConnectorTypeID is the type identifier for AppConnector objects.
	AppConnectorTypeID = "spacewave/app-connector"
	// AppSnapshotTypeID is the type identifier for AppSnapshot objects.
	AppSnapshotTypeID = "spacewave/app-snapshot"
	// DefaultPollIntervalMs is the poll interval applied by create paths.
	DefaultPollIntervalMs uint32 = 60_000
	// DefaultMaxBodyBytes is the response body limit applied by create paths.
	DefaultMaxBodyBytes uint32 = 1 << 20
	// SnapshotKeySuffix is appended to a connector key to name its snapshot.
	SnapshotKeySuffix = "/snapshot"
)

// PredAppConnectorSnapshot links an AppConnector to its AppSnapshot.
var PredAppConnectorSnapshot = quad.IRI("spacewave/app-connector/snapshot")

// NewAppConnectorToSnapshotQuad creates a quad linking an AppConnector to its AppSnapshot.
func NewAppConnectorToSnapshotQuad(connectorKey, snapshotKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		connectorKey,
		PredAppConnectorSnapshot.String(),
		snapshotKey,
		"",
	)
}

// SnapshotObjectKey returns the key of the AppSnapshot created with a connector.
func SnapshotObjectKey(connectorKey string) string {
	return connectorKey + SnapshotKeySuffix
}

// LookupSnapshotKey returns the key of the AppSnapshot linked from a connector.
func LookupSnapshotKey(ctx context.Context, ws world.WorldStateGraph, connectorKey string) (string, error) {
	// Find the connector's snapshot edge.
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(
		connectorKey,
		PredAppConnectorSnapshot.String(),
		"",
		"",
	), 1)
	if err != nil {
		return "", err
	}
	if len(quads) == 0 {
		return "", errors.Wrap(world.ErrObjectNotFound, "app connector snapshot")
	}

	// Decode the snapshot key from the edge object.
	return world.GraphValueToKey(quads[0].GetObj())
}

// NewAppConnectorBlock constructs a new AppConnector block.
func NewAppConnectorBlock() block.Block {
	return &AppConnector{}
}

// MarshalBlock marshals the AppConnector to bytes.
func (c *AppConnector) MarshalBlock() ([]byte, error) {
	return c.MarshalVT()
}

// UnmarshalBlock unmarshals the AppConnector from bytes.
func (c *AppConnector) UnmarshalBlock(data []byte) error {
	return c.UnmarshalVT(data)
}

// Validate performs cursory checks on the AppConnector block.
func (c *AppConnector) Validate() error {
	// Require a label and a Secret holding the API token.
	if strings.TrimSpace(c.GetLabel()) == "" {
		return errors.New("app connector label is required")
	}
	if c.GetTokenSecretObjectKey() == "" {
		return errors.New("app connector token secret is required")
	}

	// Require an HTTP origin to read from.
	baseURL, err := url.Parse(c.GetBaseUrl())
	if err != nil {
		return errors.Wrap(err, "app connector base url")
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return errors.New("app connector base url must be http or https")
	}
	if baseURL.Host == "" {
		return errors.New("app connector base url host is required")
	}

	// Require a poll interval and a response size limit.
	if c.GetPollIntervalMs() == 0 {
		return errors.New("app connector poll interval is required")
	}
	if c.GetMaxBodyBytes() == 0 {
		return errors.New("app connector max body bytes is required")
	}

	// Require at least one read, each uniquely named with an absolute path.
	if len(c.GetReads()) == 0 {
		return errors.New("app connector reads are required")
	}
	names := make(map[string]struct{}, len(c.GetReads()))
	for _, read := range c.GetReads() {
		if read.GetName() == "" {
			return errors.New("app connector read name is required")
		}
		if _, ok := names[read.GetName()]; ok {
			return errors.Errorf("app connector read %q is duplicated", read.GetName())
		}
		names[read.GetName()] = struct{}{}
		if !strings.HasPrefix(read.GetPath(), "/") {
			return errors.Errorf("app connector read %q path must start with /", read.GetName())
		}
	}
	return nil
}

// NewAppSnapshotBlock constructs a new AppSnapshot block.
func NewAppSnapshotBlock() block.Block {
	return &AppSnapshot{}
}

// UnmarshalAppSnapshot unmarshals an AppSnapshot from a cursor.
func UnmarshalAppSnapshot(ctx context.Context, bcs *block.Cursor) (*AppSnapshot, error) {
	return block.UnmarshalBlock[*AppSnapshot](ctx, bcs, NewAppSnapshotBlock)
}

// MarshalBlock marshals the AppSnapshot to bytes.
func (s *AppSnapshot) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the AppSnapshot from bytes.
func (s *AppSnapshot) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

var (
	_ block.Block = ((*AppConnector)(nil))
	_ block.Block = ((*AppSnapshot)(nil))
)
