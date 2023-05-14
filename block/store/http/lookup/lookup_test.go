package block_store_http_lookup

import (
	"bytes"
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aperturerobotics/bifrost/hash"
	httplog "github.com/aperturerobotics/bifrost/http/log"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/block"
	block_store_http_server "github.com/aperturerobotics/hydra/block/store/http/server"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	lookup_concurrent "github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreHTTPLookup tests the block store http lookup controller.
func TestBlockStoreHTTPLookup(t *testing.T) {
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
	handler := block_store_http_server.NewHTTPBlock(serverVol, true, blockStorePrefix, 0)
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
	clientTb.StaticResolver.AddFactory(NewFactory(clientTb.Bus))

	// Create the bucket in the client
	bucketID := clientTb.BucketId
	// override the bucket config with v2
	bucketLkConfig, err := bucket.NewLookupConfig(configset.NewControllerConfig(1, &lookup_concurrent.Config{
		// enable looking up via directive
		NotFoundBehavior:  lookup_concurrent.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE,
		WritebackBehavior: lookup_concurrent.WritebackBehavior_WritebackBehavior_ALL_VOLUMES,
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

	// Create the lookup controller
	_, _, lookupCtrlRel, err := loader.WaitExecControllerRunning(
		ctx,
		clientTb.Bus,
		resolver.NewLoadControllerWithConfig(NewConfig(bucketID, baseURL.String())),
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
	readBkt, _, readBktRef, err := volume.ExBuildBucketAPI(ctx, clientTb.Bus, false, bucketID, clientTb.Volume.GetID(), nil)
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
