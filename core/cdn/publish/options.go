package publish

import (
	"context"
	"io"
	"os"

	"github.com/pkg/errors"
	spacewave_provider "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/sirupsen/logrus"
)

// SessionClient is the authenticated Spacewave client surface needed to publish.
type SessionClient interface {
	// ReadPack streams a whole pack through the authenticated service.
	ReadPack(ctx context.Context, resourceID, packID string) (io.ReadCloser, error)
	// OpenPackReader opens ranged reads of a pack of known size.
	OpenPackReader(resourceID, packID string, size int64) (*packfile_store.PackReader, error)
	// GetSOState reads the authoritative shared object state.
	GetSOState(ctx context.Context, soID string, since uint64, reason spacewave_provider.SeedReason) ([]byte, error)
	// SyncPull lists the block store's pack catalog after since.
	SyncPull(ctx context.Context, resourceID string, since uint64) (*packfile.PullResponse, error)
	// SyncPushData uploads a pack to the block store.
	SyncPushData(
		ctx context.Context,
		resourceID string, packID string, blockCount int,
		packData []byte, bodyHash []byte, bloomFilter []byte,
		bloomFormatVersion uint32,
	) error
	// SyncReplaceData uploads a pack that atomically supersedes up to 32 packs.
	SyncReplaceData(
		ctx context.Context,
		resourceID, packID string, blockCount int,
		packData, bodyHash, bloomFilter []byte,
		bloomFormatVersion uint32, replacedPackIDs []string,
	) error
	// PostCheckpoint publishes a signed checkpoint of the shared object.
	PostCheckpoint(ctx context.Context, soID string, checkpoint *sobject.SOCheckpoint) error
}

// TempFileFactory creates a temporary file for a staged pack.
type TempFileFactory func(pattern string) (*os.File, error)

// Options supplies per-operation dependencies and endpoints for CDN publication.
// Publication functions and higher-level mirror and release workflows share these
// settings. The caller owns the authenticated session; Options does not construct
// a service or retain publication state.
type Options struct {
	// Client is the caller-supplied authenticated session for pack and
	// checkpoint requests. Publication does not replace or close it, so callers
	// can reuse the session across operations.
	Client SessionClient
	// Logger receives progress, or nil.
	Logger *logrus.Entry
	// Output receives per-pack lines, or nil to discard them.
	Output io.Writer
	// Endpoint is the service base URL.
	Endpoint string
	// CdnBaseURL is the public base URL of the destination's packs.
	CdnBaseURL string
	// SrcSpaceID is the Space packs are copied from, when copying.
	SrcSpaceID string
	// DstSpaceID is the Space being published.
	DstSpaceID string
	// OwnerKeyPem is the path of the PEM key that signs checkpoints.
	OwnerKeyPem string
	// TempFileFactory creates staged pack files, or nil for os.CreateTemp.
	TempFileFactory TempFileFactory
	// KeepPacks keeps every published pack: a publish only adds packs and
	// Reclaim refuses to run, for a destination that deletes no pack.
	KeepPacks bool
}

// output selects the per-pack output stream.
func (o Options) output() io.Writer {
	if o.Output != nil {
		return o.Output
	}
	return io.Discard
}

// tempFileFactory selects the staged-pack file factory.
func (o Options) tempFileFactory() TempFileFactory {
	if o.TempFileFactory != nil {
		return o.TempFileFactory
	}
	return func(pattern string) (*os.File, error) {
		return os.CreateTemp("", pattern)
	}
}

// WithDestination binds publication to spaceID, defaulting its public CDN URL.
// An explicitly selected different Space is rejected before publication.
func (o Options) WithDestination(spaceID, cdnBaseURL string) (Options, error) {
	// Fill destination settings the caller left unspecified.
	if o.DstSpaceID == "" {
		o.DstSpaceID = spaceID
	}
	if o.CdnBaseURL == "" {
		o.CdnBaseURL = cdnBaseURL
	}

	// Require publication to target the selected Space.
	if o.DstSpaceID != spaceID {
		return Options{}, errors.Errorf("publication Space %q is not the deployment's %q", o.DstSpaceID, spaceID)
	}
	return o, nil
}

// Log reports publication progress when a logger is configured.
func (o Options) Log(level logrus.Level, fields logrus.Fields, message string) {
	if o.Logger != nil {
		o.Logger.WithFields(fields).Log(level, message)
	}
}
