package sobject_world_engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/util/blockenc"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// TestDefaultWorldTransformEncryptsStoredWorldBlock checks that the default
// World transform encrypts the blocks it writes and authenticates them.
func TestDefaultWorldTransformEncryptsStoredWorldBlock(t *testing.T) {
	// Name a plaintext marker to look for in stored blocks.
	const marker = "spacewave-world-plaintext-regression-marker-20260811"
	ctx := context.Background()

	// Build the default World transform.
	initOp, err := NewInitWorldOp(nil)
	if err != nil {
		t.Fatalf("NewInitWorldOp: %v", err)
	}
	state, err := BuildInitialInnerState(initOp)
	if err != nil {
		t.Fatalf("BuildInitialInnerState: %v", err)
	}
	conf := state.GetHeadRef().GetTransformConf()
	sfs := transform_all.BuildFactorySet()
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, sfs, conf)
	if err != nil {
		t.Fatalf("NewTransformer: %v", err)
	}

	// Write a World object that holds the marker.
	store := block_mock.NewMockStore(0)
	tx, cursor := block.NewTransaction(store, xfrm, nil, nil)
	cursor.SetBlock(world_block.NewObject(marker, nil), true)
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatalf("write world block: %v", err)
	}

	// The stored block hides the marker.
	raw, found, err := store.GetBlock(ctx, ref)
	if err != nil {
		t.Fatalf("read stored world block: %v", err)
	}
	if !found {
		t.Fatal("stored world block not found")
	}
	if bytes.Contains(raw, []byte(marker)) {
		t.Fatal("stored world block contains plaintext marker")
	}

	// Gzip alone does not reveal it.
	if recovered, err := gunzip(raw); err == nil && bytes.Contains(recovered, []byte(marker)) {
		t.Fatal("stored world block is recoverable without the participant transform key")
	}

	// The participant key decodes the object.
	decoded, err := xfrm.DecodeBlock(raw)
	if err != nil {
		t.Fatalf("decode with participant transform key: %v", err)
	}
	obj := &world_block.Object{}
	if err := obj.UnmarshalBlock(decoded); err != nil {
		t.Fatalf("unmarshal decrypted world block: %v", err)
	}
	if obj.GetKey() != marker {
		t.Fatalf("decrypted world object key = %q, want %q", obj.GetKey(), marker)
	}

	// Tampering fails authentication.
	tampered := bytes.Clone(raw)
	tampered[len(tampered)-1] ^= 1
	if _, err := xfrm.DecodeBlock(tampered); err == nil {
		t.Fatal("tampered world block decrypted without authentication failure")
	}

	// A key that differs in one bit cannot decode the block.
	wrongConf := conf.CloneVT()
	foundEncryption := false
	for i, step := range wrongConf.GetSteps() {
		if step.GetId() != transform_blockenc.ConfigID {
			continue
		}
		encConf := &transform_blockenc.Config{}
		if err := block_transform.UnmarshalStepConfig(step.GetConfig(), encConf); err != nil {
			t.Fatalf("unmarshal participant encryption config: %v", err)
		}
		if encConf.GetBlockEnc() != blockenc.DefaultBlockEnc {
			t.Fatalf("world block encryption = %s, want %s", encConf.GetBlockEnc(), blockenc.DefaultBlockEnc)
		}
		encConf.Key = bytes.Clone(encConf.GetKey())
		encConf.Key[0] ^= 1
		wrongConf.Steps[i], err = block_transform.NewStepConfig(encConf)
		if err != nil {
			t.Fatalf("build wrong-key config: %v", err)
		}
		foundEncryption = true
	}
	if !foundEncryption {
		t.Fatal("default world transform has no authenticated encryption step")
	}
	wrongXfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, sfs, wrongConf)
	if err != nil {
		t.Fatalf("build wrong-key transformer: %v", err)
	}
	if _, err := wrongXfrm.DecodeBlock(raw); err == nil {
		t.Fatal("world block decrypted with the wrong participant transform key")
	}
}

// TestDefaultWorldTransformDecodesNativeCiphertext checks that the default
// transform decodes a native ciphertext fixture.
func TestDefaultWorldTransformDecodesNativeCiphertext(t *testing.T) {
	const ciphertextHex = "592821e714255f798541eaff788680b7aa65868bad9da8baf606d315dd58e31c5cbd447cbba2ddeb9ebbeba71761bf6164b1113436ca77f190fb22a3075540aa23d0ebe9"
	key := []byte("0123456789abcdef0123456789abcdef")
	conf, err := block_transform.NewConfig([]config.Config{&transform_blockenc.Config{
		BlockEnc: blockenc.DefaultBlockEnc,
		Key:      key,
	}})
	if err != nil {
		t.Fatalf("build native fixture config: %v", err)
	}
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, transform_all.BuildFactorySet(), conf)
	if err != nil {
		t.Fatalf("build native fixture transformer: %v", err)
	}
	ciphertext, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		t.Fatalf("decode native ciphertext fixture: %v", err)
	}
	plaintext, err := xfrm.DecodeBlock(ciphertext)
	if err != nil {
		t.Fatalf("decode native ciphertext: %v", err)
	}
	if string(plaintext) != "native-goscript-world-ciphertext-fixture" {
		t.Fatalf("decoded native ciphertext = %q", plaintext)
	}
}

// TestUnauthenticatedWorldInitializationIsRejected checks that a World cannot
// start with an unauthenticated transform.
func TestUnauthenticatedWorldInitializationIsRejected(t *testing.T) {
	conf, err := block_transform.NewConfig([]config.Config{&transform_gzip.Config{}})
	if err != nil {
		t.Fatalf("build unauthenticated transform config: %v", err)
	}
	_, err = BuildInitialInnerState(&InitWorldOp{TransformConf: conf})
	if !errors.Is(err, errUnauthenticatedWorldTransform) {
		t.Fatalf("world initialization error = %v, want %v", err, errUnauthenticatedWorldTransform)
	}
}

// TestUnauthenticatedWorldTransformerIsRejected checks that a World with an
// unauthenticated transform does not load.
func TestUnauthenticatedWorldTransformerIsRejected(t *testing.T) {
	conf, err := block_transform.NewConfig([]config.Config{&transform_gzip.Config{}})
	if err != nil {
		t.Fatalf("build unauthenticated transform config: %v", err)
	}
	_, err = newWorldTransformer(controller.ConstructOpts{}, transform_all.BuildFactorySet(), conf)
	if !errors.Is(err, errUnauthenticatedWorldTransform) {
		t.Fatalf("world transformer error = %v, want %v", err, errUnauthenticatedWorldTransform)
	}
}

// gunzip decompresses data.
func gunzip(data []byte) ([]byte, error) {
	rd, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer rd.Close()
	return io.ReadAll(rd)
}
