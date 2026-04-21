package block_store_http_server

import (
	"bytes"
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	httplog "github.com/aperturerobotics/util/httplog"
	"github.com/s4wave/spacewave/db/block"
	block_store_http "github.com/s4wave/spacewave/db/block/store/http"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	bifrost_http "github.com/s4wave/spacewave/net/http"
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
	sampleBlockRef, _, err := vol.PutBlock(ctx, sampleBlockBody, samplePutOpts)
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
	client := block_store_http.NewHTTPBlock(le, true, srv.Client(), baseURL, 0, true)

	// Get the sample block
	retBlockData, retBlockExists, err := client.GetBlock(ctx, sampleBlockRef)
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
	retBlockExists, err = client.GetBlockExists(ctx, sampleBlockRef)
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
	retBlockData, retBlockExists, err = client.GetBlock(ctx, sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists || len(retBlockData) != 0 {
		t.Fail()
	}

	// Check if the block exists (not expected to)
	retBlockExists, err = client.GetBlockExists(ctx, sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}

	// Put the block
	samplePutOpts2 := samplePutOpts.CloneVT()
	samplePutOpts2.ForceBlockRef = sampleBlockRef2.Clone()
	ref, existed, err := client.PutBlock(ctx, sampleBlockBody2, samplePutOpts2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := ref.Validate(false); err != nil {
		t.Fatal(err.Error())
	}
	if !ref.EqualsRef(sampleBlockRef2) {
		t.Fail()
	}
	if existed {
		t.Fail()
	}

	// Get the block back again
	retBlockData, retBlockExists, err = client.GetBlock(ctx, sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !retBlockExists || !bytes.Equal(retBlockData, sampleBlockBody2) {
		t.Fail()
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(ctx, sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !retBlockExists {
		t.Fail()
	}

	// Delete the block
	err = client.RmBlock(ctx, sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(ctx, sampleBlockRef2)
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
	client := block_store_http.NewHTTPBlock(le, true, srv.Client(), baseURL, 0, true)

	// Create a sample block
	sampleBlockBody := []byte("No, I'm just reading. Yep. Machiavelli, pretty simple book yeah.")
	samplePutOpts := &block.PutOpts{HashType: hash.HashType_HashType_SHA256}
	sampleBlockRef, err := block.BuildBlockRef(sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check if the block exists
	retBlockExists, err := client.GetBlockExists(ctx, sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}

	// Put the block
	ref, existed, err := client.PutBlock(ctx, sampleBlockBody, samplePutOpts)
	if err == nil {
		t.Fatal("expected PutBlock to fail")
	} else {
		le.Infof("got expected failure to put block on read only store: %s", err.Error())
	}
	if ref != nil || existed {
		t.Fail()
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(ctx, sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}
}

// TestBlockStoreHTTPServer_Controller tests the http server controller.
func TestBlockStoreHTTPServer_Controller(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	// Create the HTTP server handler
	blockStorePrefix := "/block"
	blockStoreID := tb.Volume.GetID()
	ctrlConf := NewConfig(blockStoreID, true, blockStorePrefix, 0)
	_, _, ctrlRef, err := loader.WaitExecControllerRunning(ctx, tb.Bus, resolver.NewLoadControllerWithConfig(ctrlConf), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ctrlRef.Release()

	// Create the HTTP server controller
	handler := bifrost_http.NewBusHandler(tb.Bus, "test-client", true)
	srv := httptest.NewServer(httplog.LoggingMiddleware(handler, le, httplog.LoggingMiddlewareOpts{UserAgent: true}))
	defer srv.Close()
	baseURL, _ := url.Parse(srv.URL)
	baseURL = baseURL.JoinPath(blockStorePrefix)

	// Create the client
	client := block_store_http.NewHTTPBlock(le, true, srv.Client(), baseURL, 0, true)

	// Create a sample block
	sampleBlockBody := []byte("No, I'm just reading. Yep. Machiavelli, pretty simple book yeah.")
	samplePutOpts := &block.PutOpts{HashType: hash.HashType_HashType_SHA256}
	sampleBlockRef, err := block.BuildBlockRef(sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check if the block exists
	retBlockExists, err := client.GetBlockExists(ctx, sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}

	// Put the block
	ref, existed, err := client.PutBlock(ctx, sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ref.GetEmpty() || existed {
		t.Fail()
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(ctx, sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !retBlockExists {
		t.Fail()
	}
}

// TestBlockStoreHTTPAsFallback tests the block store http controller.
func TestBlockStoreHTTPAsFallback(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	serverTb, err := testbed.NewTestbed(ctx, le.WithField("testbed", "server"))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a block to lookup.
	serverVol := serverTb.Volume
	sampleBlockBody := []byte("How hard are these tests? What exactly was in that phonebook of a contract I signed?")
	samplePutOpts := &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3}
	sampleBlockRef, _, err := serverVol.PutBlock(ctx, sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("put sample block ref %v", sampleBlockRef.MarshalString())

	// Create the HTTP server
	blockStorePrefix := "/block-store"
	handler := NewHTTPBlock(serverVol, true, blockStorePrefix, 0)
	srv := httptest.NewServer(httplog.LoggingMiddleware(handler, le, httplog.LoggingMiddlewareOpts{
		UserAgent: true,
	}))
	defer srv.Close()
	baseURL, _ := url.Parse(srv.URL)
	baseURL = baseURL.JoinPath(blockStorePrefix)

	// Create the client
	clientTb, err := testbed.NewTestbed(ctx, le.WithField("testbed", "client"))
	if err != nil {
		t.Fatal(err.Error())
	}
	clientTb.StaticResolver.AddFactory(block_store_http.NewFactory(clientTb.Bus))

	// Create the bucket in the client
	bucketID := clientTb.BucketId
	// override the bucket config with v2
	blockStoreID := "test/http-block-store"
	bucketLkConfig, err := bucket.NewLookupConfig(configset.NewControllerConfig(1, &lookup_concurrent.Config{
		// Use FallbackBlockStoreId
		FallbackBlockStoreId: blockStoreID,
		NotFoundBehavior:     lookup_concurrent.NotFoundBehavior_NotFoundBehavior_NONE,
		WritebackBehavior:    lookup_concurrent.WritebackBehavior_WritebackBehavior_ALL,
	}))
	if err != nil {
		t.Fatal(err.Error())
	}
	bucketConf, err := bucket.NewConfig(bucketID, 2, nil, bucketLkConfig)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, err = bucket.ExApplyBucketConfig(ctx, clientTb.Bus, bucket.NewApplyBucketConfig(bucketConf, nil, []string{clientTb.Volume.GetID()}))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the http block store controller
	conf := block_store_http.NewConfig(blockStoreID, baseURL.String(), true, nil)
	conf.Verbose = true
	_, _, lookupCtrlRel, err := loader.WaitExecControllerRunning(
		ctx,
		clientTb.Bus,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lookupCtrlRel.Release()

	// Create the bucket lookup handle
	lkr, _, lkRef, err := bucket_lookup.ExBuildBucketLookup(ctx, clientTb.Bus, false, bucketID, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lkRef.Release()

	lk, err := lkr.GetLookup(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	lkDat, lkFound, err := lk.LookupBlock(ctx, sampleBlockRef.Clone())
	if err != nil {
		t.Fatal(err.Error())
	}

	if !lkFound {
		t.FailNow()
	}
	if !bytes.Equal(lkDat, sampleBlockBody) {
		t.FailNow()
	}

	// check if write-back worked
	readBkt, _, readBktRef, err := bucket.ExBuildBucketAPI(ctx, clientTb.Bus, false, bucketID, clientTb.Volume.GetID(), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readBktRef.Release()

	ex, err := readBkt.GetBucket().GetBlockExists(ctx, sampleBlockRef.Clone())
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ex {
		t.Fatal("expected to write back block to bucket but did not")
	}
}
