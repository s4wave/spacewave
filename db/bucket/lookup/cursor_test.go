package bucket_lookup

import (
	"bytes"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
)

func TestTransformConfEnvelopeRoundTrip(t *testing.T) {
	conf := testTransformConf(t)
	encoded, err := MarshalTransformConf(conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) < transformConfEnvelopeHeaderSize ||
		!bytes.Equal(encoded[:len(transformConfEnvelopeMagic)], []byte(transformConfEnvelopeMagic)) ||
		encoded[len(transformConfEnvelopeMagic)] != transformConfEnvelopeVersion {
		t.Fatalf("unexpected transform config envelope: %x", encoded)
	}

	decoded, err := UnmarshalTransformConf(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.EqualVT(conf) {
		t.Fatalf("decoded config mismatch: got %v, want %v", decoded, conf)
	}
}

func TestTransformConfLegacyCRC32RoundTrip(t *testing.T) {
	conf := testTransformConf(t)
	payload, err := conf.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := transform_chksum.EncodeCRC32(payload)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := UnmarshalTransformConf(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.EqualVT(conf) {
		t.Fatalf("decoded legacy config mismatch: got %v, want %v", decoded, conf)
	}

	legacy[len(legacy)-1] ^= 0xff
	if _, err := UnmarshalTransformConf(legacy); err == nil {
		t.Fatal("corrupt legacy transform config accepted")
	}
}

func TestTransformConfEnvelopeRejectsUnknownVersion(t *testing.T) {
	encoded, err := MarshalTransformConf(testTransformConf(t))
	if err != nil {
		t.Fatal(err)
	}
	encoded[len(transformConfEnvelopeMagic)]++
	if _, err := UnmarshalTransformConf(encoded); err == nil {
		t.Fatal("unknown transform config envelope version accepted")
	}
}

func testTransformConf(t *testing.T) *block_transform.Config {
	t.Helper()
	conf, err := block_transform.NewConfig([]config.Config{&transform_s2.Config{Better: true}})
	if err != nil {
		t.Fatal(err)
	}
	return conf
}
