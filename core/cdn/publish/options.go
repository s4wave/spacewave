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
	ReadGrants(ctx context.Context, resourceID string, packIDs []string) ([]*packfile.ReadGrant, error)
	OpenPackReader(resourceID, packID string, size int64) (*packfile_store.PackReader, error)
	GetSOState(ctx context.Context, soID string, since uint64, reason spacewave_provider.SeedReason) ([]byte, error)
	SyncPull(ctx context.Context, resourceID string, since uint64) (*packfile.PullResponse, error)
	SyncPushData(ctx context.Context, resourceID string, packID string, blockCount int, packData []byte, bodyHash []byte, bloomFilter []byte, bloomFormatVersion uint32) error
	PostCheckpoint(ctx context.Context, soID string, checkpoint *sobject.SOCheckpoint) error
}

// TempFileFactory creates a temporary file for a staged pack.
type TempFileFactory func(pattern string) (*os.File, error)

// Options carries dependencies and endpoints for CDN Space publication.
type Options struct {
	Client          SessionClient
	Logger          *logrus.Entry
	Output          io.Writer
	Endpoint        string
	CdnBaseURL      string
	SrcSpaceID      string
	DstSpaceID      string
	OwnerKeyPem     string
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
