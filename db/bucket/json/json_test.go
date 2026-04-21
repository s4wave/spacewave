package bucket_json

import (
	"testing"
	"time"

	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/db/block"
	reconciler_example "github.com/s4wave/spacewave/db/reconciler/example"
)

func TestParseConfig(t *testing.T) {
	conf, err := ParseConfig([]byte(`{
		"id":"bucket-1",
		"version":7,
		"reconcilers":[{
			"id":"reconciler-1",
			"controller":{
				"id":"hydra/reconciler/example",
				"rev":3,
				"config":{
					"bucketId":"bucket-1",
					"blockStoreId":"store-1",
					"reconcilerId":"reconciler-1"
				}
			}
		}],
		"putOpts":{},
		"lookup":{"disable":true}
	}`))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if conf.Id != "bucket-1" {
		t.Fatalf("unexpected id: %q", conf.Id)
	}
	if conf.Rev != 7 {
		t.Fatalf("unexpected version: %d", conf.Rev)
	}
	if len(conf.Reconcilers) != 1 {
		t.Fatalf("unexpected reconciler count: %d", len(conf.Reconcilers))
	}
	if conf.Reconcilers[0].Id != "reconciler-1" {
		t.Fatalf("unexpected reconciler id: %q", conf.Reconcilers[0].Id)
	}
	if conf.Reconcilers[0].Controller == nil {
		t.Fatal("expected reconciler controller")
	}
	if conf.Reconcilers[0].Controller.Id != reconciler_example.ControllerID {
		t.Fatalf("unexpected controller id: %q", conf.Reconcilers[0].Controller.Id)
	}
	if conf.Reconcilers[0].Controller.Rev != 3 {
		t.Fatalf("unexpected controller revision: %d", conf.Reconcilers[0].Controller.Rev)
	}
	if conf.PutOpts == nil {
		t.Fatal("expected put options")
	}
	if conf.Lookup == nil || !conf.Lookup.Disable {
		t.Fatalf("unexpected lookup config: %+v", conf.Lookup)
	}
}

func TestMarshalApplyBucketConfigResult(t *testing.T) {
	conf := &Config{
		Id:      "bucket-1",
		Rev:     7,
		PutOpts: &block.PutOpts{},
		Reconcilers: []ReconcilerConfig{
			{
				Id: "reconciler-1",
				Controller: &configset_json.ControllerConfig{
					Rev: 3,
					Id:  reconciler_example.ControllerID,
					Config: configset_json.NewConfig(&reconciler_example.Config{
						BucketId:     "bucket-1",
						BlockStoreId: "store-1",
						ReconcilerId: "reconciler-1",
					}),
				},
			},
		},
		Lookup: &LookupConfig{Disable: true},
	}
	result := &ApplyBucketConfigResult{
		BucketId:   "bucket-1",
		VolumeId:   "volume-1",
		BucketConf: conf,
		Timestamp:  time.Date(2026, time.April, 12, 9, 8, 7, 0, time.UTC),
		Updated:    true,
	}

	dat, err := result.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	var p fastjson.Parser
	v, err := p.ParseBytes(dat)
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}

	if got := string(v.GetStringBytes("bucket_id")); got != "bucket-1" {
		t.Fatalf("unexpected bucket id: %q", got)
	}
	if got := string(v.GetStringBytes("volume_id")); got != "volume-1" {
		t.Fatalf("unexpected volume id: %q", got)
	}
	if got := v.GetUint("bucket_conf", "version"); got != 7 {
		t.Fatalf("unexpected config version: %d", got)
	}
	if !v.GetBool("bucket_conf", "lookup", "disable") {
		t.Fatal("expected lookup disable flag")
	}
	if got := string(v.GetStringBytes("bucket_conf", "reconcilers", "0", "controller", "config", "bucketId")); got != "bucket-1" {
		t.Fatalf("unexpected controller config bucketId: %q", got)
	}
	if got := string(v.GetStringBytes("timestamp")); got != "2026-04-12T09:08:07Z" {
		t.Fatalf("unexpected timestamp: %q", got)
	}
	if !v.GetBool("updated") {
		t.Fatal("expected updated flag")
	}
}
