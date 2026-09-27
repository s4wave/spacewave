//go:build !js

package bldr_web_bundler_vite

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	frontend "github.com/s4wave/spacewave/bldr/frontend"
	buildbudget "github.com/s4wave/spacewave/bldr/util/buildbudget"
)

// BudgetClient admits expensive compiler operations without reserving idle capacity.
type BudgetClient struct {
	// client is the compiler process's RPC client.
	client SRPCViteBundlerClient
	// budget admits active compilation and initial dependency preparation.
	budget *buildbudget.Budget
}

// NewBudgetClient wraps a compiler client with the shared build budget.
func NewBudgetClient(client SRPCViteBundlerClient) (*BudgetClient, error) {
	budget, err := buildbudget.Default()
	if err != nil {
		return nil, err
	}
	return &BudgetClient{client: client, budget: budget}, nil
}

// SRPCClient returns the underlying transport client.
func (c *BudgetClient) SRPCClient() srpc.Client { return c.client.SRPCClient() }

// Build admits one complete bundle operation and releases capacity on return.
func (c *BudgetClient) Build(ctx context.Context, request *BuildRequest) (*BuildResponse, error) {
	permit, err := c.budget.Acquire(ctx, buildbudget.ViteBuildWeight)
	if err != nil {
		return nil, err
	}
	defer permit.Release()
	return c.client.Build(ctx, request)
}

// BuildWebPkg admits one shared-package build and releases capacity on return.
func (c *BudgetClient) BuildWebPkg(ctx context.Context, request *BuildWebPkgRequest) (*BuildWebPkgResponse, error) {
	permit, err := c.budget.Acquire(ctx, buildbudget.ViteBuildWeight)
	if err != nil {
		return nil, err
	}
	defer permit.Release()
	return c.client.BuildWebPkg(ctx, request)
}

// StartDevelopment admits environment setup and dependency preparation only.
func (c *BudgetClient) StartDevelopment(ctx context.Context, request *DevelopmentConfig) (*DevelopmentResult, error) {
	permit, err := c.budget.Acquire(ctx, buildbudget.ViteBuildWeight)
	if err != nil {
		return nil, err
	}
	defer permit.Release()
	return c.client.StartDevelopment(ctx, request)
}

// WatchDevelopment retains no compilation permit while waiting for changes.
func (c *BudgetClient) WatchDevelopment(ctx context.Context, request *frontend.WatchRequest) (SRPCViteBundler_WatchDevelopmentClient, error) {
	return c.client.WatchDevelopment(ctx, request)
}

// SendDevelopment delivers a client event without reserving compilation capacity.
func (c *BudgetClient) SendDevelopment(ctx context.Context, request *frontend.SendRequest) (*frontend.SendResponse, error) {
	return c.client.SendDevelopment(ctx, request)
}

// ChangeDevelopment reports source edits without reserving compilation capacity.
func (c *BudgetClient) ChangeDevelopment(ctx context.Context, request *ChangeDevelopmentRequest) (*ChangeDevelopmentResponse, error) {
	return c.client.ChangeDevelopment(ctx, request)
}

// _ is a type assertion.
var _ SRPCViteBundlerClient = (*BudgetClient)(nil)
