package block_store_http_server

import (
	"bytes"
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aperturerobotics/bifrost/hash"
	httplog "github.com/aperturerobotics/bifrost/http/log"
	"github.com/aperturerobotics/hydra/block"
	block_store_http "github.com/aperturerobotics/hydra/block/store/http"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreHTTPServer tests the block store http server and client.
func TestBlockStoreHTTPServer(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a block to lookup.
	vol := tb.Volume
	sampleBlockBody := []byte("How hard are these tests? What exactly was in that phonebook of a contract I signed?")
	samplePutOpts := &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3}
	sampleBlockRef, _, err := vol.PutBlock(sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("put sample block ref %v", sampleBlockRef.MarshalString())

	// Create the HTTP server
	blockStorePrefix := "/block-store"
	handler := NewHTTPBlock(vol, true, blockStorePrefix, 0)
	srv := httptest.NewServer(httplog.LoggingMiddleware(handler, le, httplog.LoggingMiddlewareOpts{
		UserAgent: true,
	}))
	defer srv.Close()
	baseURL, _ := url.Parse(srv.URL)
	baseURL = baseURL.JoinPath(blockStorePrefix)

	// Create the client
	client := block_store_http.NewHTTPBlock(ctx, true, srv.Client(), baseURL, 0)

	// Get the sample block
	retBlockData, retBlockExists, err := client.GetBlock(sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(retBlockData, sampleBlockBody) {
		t.Fail()
	}
	if !retBlockExists {
		t.Fail()
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !retBlockExists {
		t.Fail()
	}

	// Get a not-found block
	// Returns a 404 error, but this is processed to exists=false.
	sampleBlockBody2 := []byte("I seem to be getting a distress signal from that aerial faith plate...")
	sampleBlockRef2, err := block.BuildBlockRef(sampleBlockBody2, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}
	retBlockData, retBlockExists, err = client.GetBlock(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists || len(retBlockData) != 0 {
		t.Fail()
	}

	// Check if the block exists (not expected to)
	retBlockExists, err = client.GetBlockExists(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}

	// Put the block
	samplePutOpts2 := samplePutOpts.CloneVT()
	samplePutOpts2.ForceBlockRef = sampleBlockRef2.Clone()
	ref, existed, err := client.PutBlock(sampleBlockBody2, samplePutOpts2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := ref.Validate(); err != nil {
		t.Fatal(err.Error())
	}
	if !ref.EqualsRef(sampleBlockRef2) {
		t.Fail()
	}
	if existed {
		t.Fail()
	}

	// Get the block back again
	retBlockData, retBlockExists, err = client.GetBlock(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !retBlockExists || !bytes.Equal(retBlockData, sampleBlockBody2) {
		t.Fail()
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !retBlockExists {
		t.Fail()
	}

	// Delete the block
	err = client.RmBlock(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}
}

// TestBlockStoreHTTPServer_ReadOnly tests the read only block store http server and client.
func TestBlockStoreHTTPServer_ReadOnly(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a block to lookup.
	vol := tb.Volume

	// Create the HTTP server
	blockStorePrefix := "/read-only-block-store"
	handler := NewHTTPBlock(vol, false, blockStorePrefix, 0)
	srv := httptest.NewServer(httplog.LoggingMiddleware(handler, le, httplog.LoggingMiddlewareOpts{UserAgent: true}))
	defer srv.Close()
	baseURL, _ := url.Parse(srv.URL)
	baseURL = baseURL.JoinPath(blockStorePrefix)

	// Create the client
	// note: we create it with write=true expecting this to fail
	client := block_store_http.NewHTTPBlock(ctx, true, srv.Client(), baseURL, 0)

	// Create a sample block
	sampleBlockBody := []byte("No, I'm just reading. Yep. Machiavelli, pretty simple book yeah.")
	samplePutOpts := &block.PutOpts{HashType: hash.HashType_HashType_SHA256}
	sampleBlockRef, err := block.BuildBlockRef(sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check if the block exists
	retBlockExists, err := client.GetBlockExists(sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}

	// Put the block
	ref, existed, err := client.PutBlock(sampleBlockBody, samplePutOpts)
	if err == nil {
		t.Fatal("expected PutBlock to fail")
	} else {
		le.Infof("got expected failure to put block on read only store: %s", err.Error())
	}
	if ref != nil || existed {
		t.Fail()
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}
}
