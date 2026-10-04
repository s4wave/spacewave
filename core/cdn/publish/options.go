package publish

import (
	"context"
	"io"
	"os"

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
	SyncPushData(ctx context.Context, resourceID string, packID string, blockCount int, packData []byte, bodyHash []byte, bloomFilter []byte, bloomFormatVersion uint32) error
	// SyncReplaceData uploads a pack that atomically supersedes up to 32 packs.
	SyncReplaceData(ctx context.Context, resourceID, packID string, blockCount int, packData, bodyHash, bloomFilter []byte, bloomFormatVersion uint32, replacedPackIDs []string) error
	// PostCheckpoint publishes a signed checkpoint of the shared object.
	PostCheckpoint(ctx context.Context, soID string, checkpoint *sobject.SOCheckpoint) error
}

// TempFileFactory creates a temporary file for a staged pack.
type TempFileFactory func(pattern string) (*os.File, error)

// Options carries dependencies and endpoints for CDN Space publication.
type Options struct {
	// Client authenticates every service request.
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
}

func (o Options) output() io.Writer {
	if o.Output != nil {
		return o.Output
	}
	return io.Discard
}

func (o Options) tempFileFactory() TempFileFactory {
	if o.TempFileFactory != nil {
		return o.TempFileFactory
	}
	return func(pattern string) (*os.File, error) {
		return os.CreateTemp("", pattern)
	}
}
