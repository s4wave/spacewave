package publish

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/s4wave/spacewave/core/sobject"
	spacewave_provider "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/sirupsen/logrus"
)

// SessionClient is the authenticated Spacewave client surface needed to publish.
type SessionClient interface {
	Do(req *http.Request) (*http.Response, error)
	GetSOState(ctx context.Context, soID string, since uint64, reason spacewave_provider.SeedReason) ([]byte, error)
	SyncPull(ctx context.Context, resourceID string, since string) ([]byte, error)
	SyncPushData(ctx context.Context, resourceID string, packID string, blockCount int, packData []byte, bodyHash []byte, bloomFilter []byte, bloomFormatVersion uint32) error
	PostRoot(ctx context.Context, soID string, root *sobject.SORoot, rejectedOps []*sobject.SOOperationRejection) error
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
	ValidatorKeyPem string
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
