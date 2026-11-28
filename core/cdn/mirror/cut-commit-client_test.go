package cdn_mirror_test

import (
	"context"

	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
)

// CutCommitClient fails pack commits after a fixed number succeeded.
// The mirror calls SyncPushCommit serially in pack order.
type CutCommitClient struct {
	// StagingClient performs every request against the real destination.
	cdn_publish.StagingClient
	// remaining is the number of commits allowed before the cut.
	remaining int
	// cut is the failure returned after the allowed commits.
	cut error
}

// NewCutCommitClient forwards commits successful times before returning cut.
func NewCutCommitClient(client cdn_publish.StagingClient, successful int, cut error) *CutCommitClient {
	return &CutCommitClient{StagingClient: client, remaining: successful, cut: cut}
}

// SyncPushCommit forwards each allowed commit and fails every later one.
func (c *CutCommitClient) SyncPushCommit(ctx context.Context, spaceID, packID string) error {
	// Cut before the next pack enters the catalog.
	if c.remaining == 0 {
		return c.cut
	}

	// Count only successful commits to the real destination.
	if err := c.StagingClient.SyncPushCommit(ctx, spaceID, packID); err != nil {
		return err
	}
	c.remaining--
	return nil
}

// _ is a type assertion.
var _ cdn_publish.StagingClient = (*CutCommitClient)(nil)
