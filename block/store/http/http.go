package block_store_http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/pkg/errors"
)

// HTTPBlock is a block store on top of a HTTP client and base URL prefix.
type HTTPBlock struct {
	ctx      context.Context
	write    bool
	client   *http.Client
	baseURL  *url.URL
	hashType hash.HashType
}

const (
	// GetPath is the path of the get endpoint.
	GetPath = "get"
	// PutPath is the path of the put endpoint.
	PutPath = "put"
	// ExistsPath is the path of the exists endpoint.
	ExistsPath = "exists"
	// RmPath is the path of the rm endpoint.
	RmPath = "rm"
)

// NewHTTPBlock builds a new block store on top of a HTTP service.
// The lookup path /block/{cid} will be appended to the URL path.
//
// baseURL cannot be nil
// client can be nil to use the default client
// hashType can be 0 to use the default hash type.
// if write=false, supports read operations only.
func NewHTTPBlock(ctx context.Context, write bool, client *http.Client, baseURL *url.URL, hashType hash.HashType) *HTTPBlock {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == nil {
		// this won't work, and nil url is not supported,
		// at least make sure it's not nil.
		baseURL = &url.URL{}
	}
	return &HTTPBlock{ctx: ctx, write: write, client: client, baseURL: baseURL, hashType: hashType}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (b *HTTPBlock) GetHashType() hash.HashType {
	return b.hashType
}

// PutBlock puts a block into the store.
// Stores should check if the block already exists if possible.
func (b *HTTPBlock) PutBlock(data []byte, opts *block.PutOpts) (ref *block.BlockRef, exists bool, err error) {
	if !b.write {
		return nil, false, block_store.ErrReadOnlyStore
	}

	// many stores cannot handle empty values
	// add a blanket check here to be sure
	if len(data) == 0 {
		return nil, false, block.ErrEmptyBlock
	}

	// Creating a block: /put
	putURL := b.baseURL.JoinPath(PutPath)
	body := &PutRequest{
		Data:    data,
		PutOpts: opts,
	}
	bodyDat, err := body.MarshalVT()
	if err != nil {
		return nil, false, err
	}

	req, err := http.NewRequestWithContext(b.ctx, "POST", putURL.String(), bytes.NewReader(bodyDat))
	if err != nil {
		return nil, false, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 405 {
		if len(respBody) != 0 {
			errStr := string(respBody)
			return nil, false, errors.New(errStr)
		} else {
			return nil, false, errors.Errorf("server returned error %v", resp.StatusCode)
		}
	}

	putResp := &PutResponse{}
	if err := putResp.UnmarshalVT(respBody); err != nil {
		return nil, false, err
	}
	if err := putResp.Validate(); err != nil {
		return nil, false, err
	}

	if errStr := putResp.GetErr(); errStr != "" {
		return nil, false, errors.Wrap(errors.New(errStr), "service error")
	}

	// double-check ref if ForceBlockRef if set.
	ref = putResp.GetRef()
	if ref.GetEmpty() {
		return nil, false, errors.Wrap(block.ErrEmptyBlockRef, "service error")
	}
	if !opts.GetForceBlockRef().GetEmpty() && !ref.EqualsRef(opts.GetForceBlockRef()) {
		return nil, false, errors.Wrapf(
			block.ErrBlockRefMismatch,
			"service error: %s != expected %s",
			ref.MarshalString(), opts.GetForceBlockRef().MarshalString(),
		)
	}
	return ref, putResp.GetExists(), nil
}

// GetBlock looks up a block in the store.
// Returns data, found, and any exceptional error.
func (b *HTTPBlock) GetBlock(ref *block.BlockRef) ([]byte, bool, error) {
	if ref.GetEmpty() {
		return nil, false, block.ErrEmptyBlockRef
	}

	// Getting a block: /get/{ref}
	refB58 := ref.MarshalString()
	getURL := b.baseURL.JoinPath(GetPath, refB58)

	req, err := http.NewRequestWithContext(b.ctx, "GET", getURL.String(), nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	getResp := &GetResponse{}
	if err := getResp.UnmarshalVT(respBody); err != nil {
		return nil, false, err
	}
	if errStr := getResp.GetErr(); errStr != "" {
		return nil, false, errors.Wrap(errors.New(errStr), "service returned error")
	}
	if getResp.GetNotFound() {
		return nil, false, nil
	}
	data := getResp.GetData()
	if len(data) == 0 {
		return nil, false, errors.New("service returned empty data but not found was not set")
	}

	// Verify the data matches the block ref.
	dlRef, err := block.BuildBlockRef(
		data,
		&block.PutOpts{HashType: ref.GetHash().GetHashType(), ForceBlockRef: ref},
	)
	if err != nil {
		return nil, false, err
	}
	if !dlRef.EqualsRef(ref) {
		return nil, true, errors.Wrapf(block.ErrBlockRefMismatch, "service returned %s but expected %s", dlRef.MarshalString(), ref.MarshalString())
	}

	return data, true, nil
}

// GetBlockExists checks if a block exists in the store.
// Returns found, and any exceptional error.
func (b *HTTPBlock) GetBlockExists(ref *block.BlockRef) (bool, error) {
	if ref.GetEmpty() {
		return false, block.ErrEmptyBlockRef
	}

	// Checking if a block exists: /exists/{ref}
	refB58 := ref.MarshalString()
	existsURL := b.baseURL.JoinPath(ExistsPath, refB58)

	req, err := http.NewRequestWithContext(b.ctx, "GET", existsURL.String(), nil)
	if err != nil {
		return false, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return false, err
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	existsResp := &ExistsResponse{}
	if err := existsResp.UnmarshalVT(respBody); err != nil {
		return false, err
	}
	if errStr := existsResp.GetErr(); errStr != "" {
		return false, errors.Wrap(errors.New(errStr), "service returned error")
	}
	if existsResp.GetNotFound() {
		return false, nil
	}
	return existsResp.GetExists(), nil
}

// RmBlock deletes a block from the store.
// Should not return an error if the block did not exist.
func (b *HTTPBlock) RmBlock(ref *block.BlockRef) error {
	if ref.GetEmpty() {
		return block.ErrEmptyBlockRef
	}
	if !b.write {
		return block_store.ErrReadOnlyStore
	}

	// Deleting a block: /rm/{ref}
	refB58 := ref.MarshalString()
	rmURL := b.baseURL.JoinPath(RmPath, refB58)

	req, err := http.NewRequestWithContext(b.ctx, "DELETE", rmURL.String(), nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	rmResp := &RmResponse{}
	if err := rmResp.UnmarshalVT(respBody); err != nil {
		return err
	}
	if errStr := rmResp.GetErr(); errStr != "" {
		return errors.Wrap(errors.New(errStr), "service returned error")
	}
	return nil
}

// _ is a type assertion
var _ block_store.Store = ((*HTTPBlock)(nil))
