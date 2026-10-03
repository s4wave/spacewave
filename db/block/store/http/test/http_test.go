package block_store_http_e2e

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
	block_store_http_server "github.com/s4wave/spacewave/db/block/store/http/server"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreHTTPEndToEnd tests the block store http controller end to end.
func TestBlockStoreHTTPEndToEnd(t *testing.T) {
	// Prepare the HTTP block-store test context and debug logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed that serves the source bucket.
	serverTb, err := testbed.NewTestbed(ctx, le.WithField("testbed", "server"))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer serverTb.Release()

	// Create a block to lookup in the server bucket, which records its refs.
	serverBkt, _, serverBktRef, err := bucket.ExBuildBucketAPI(ctx, serverTb.Bus, false, serverTb.BucketId, serverTb.Volume.GetID(), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer serverBktRef.Release()

	// Write a referenced sample block into the server bucket.
	serverStore := serverBkt.GetBucket()
	sampleBlockBody := []byte("How hard are these tests? What exactly was in that phonebook of a contract I signed?")
	samplePutOpts := &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3}
	sampleBlockRef, _, err := serverStore.PutBlock(ctx, sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("put sample block ref %v", sampleBlockRef.MarshalString())

	// Create the HTTP server
	blockStorePrefix := "/block-store"
	handler := block_store_http_server.NewHTTPBlock(serverStore, true, blockStorePrefix, 0)
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
	defer clientTb.Release()
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
	bucketConf, err := bucket.NewConfig(bucketID, 2, bucketLkConfig)
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

	// Acquire the configured client bucket lookup interface.
	lk, err := lkr.GetLookup(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Fetch the sample block through the HTTP-backed bucket lookup.
	lkDat, lkFound, err := lk.LookupBlock(ctx, sampleBlockRef.Clone())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the HTTP-backed lookup returns the original block bytes.
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

	// Verify the fetched block was written back into the client bucket.
	ex, err := readBkt.GetBucket().GetBlockExists(ctx, sampleBlockRef.Clone())
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ex {
		t.Fatal("expected to write back block to bucket but did not")
	}
}
