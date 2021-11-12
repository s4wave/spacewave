package block_transform_json

import (
	"encoding/json"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	transform_all "github.com/aperturerobotics/hydra/block/transform/all"
	transform_snappy "github.com/aperturerobotics/hydra/block/transform/snappy"
	"github.com/sirupsen/logrus"
)

var basicOutput = `{"steps":[{"id":"hydra/transform/snappy/1","config":{}}]}`

// TestMarshalConfig tests marshaling a config to json.
func TestMarshalConfig(t *testing.T) {
	tsConf := &transform_snappy.Config{}
	cc, err := NewConfig([]config.Config{tsConf})
	if err != nil {
		t.Fatal(err.Error())
	}
	dat, err := json.Marshal(cc)
	if err != nil {
		t.Fatal(err.Error())
	}
	v := string(dat)
	if v != basicOutput {
		t.Fatalf("unexpected output %s", v)
	}
}

// TestUnmarshalConfig tests unmarshaling a config to json.
func TestUnmarshalConfig(t *testing.T) {
	c := new(Config)
	if err := json.Unmarshal([]byte(basicOutput), c); err != nil {
		t.Fatal(err.Error())
	}

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	sfs, err := transform_all.BuildFactorySet()
	if err != nil {
		t.Fatal(err.Error())
	}
	steps, confs, err := c.Resolve(sfs, controller.ConstructOpts{Logger: le})
	if err != nil {
		t.Fatal(err.Error())
	}
	if confs[0].GetConfigID() != "hydra/transform/snappy/1" {
		t.Fail()
	}
	if len(confs) != 1 || len(steps) != 1 {
		t.Fail()
	}
}
