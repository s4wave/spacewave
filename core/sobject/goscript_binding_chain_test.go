package sobject

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
)

// Run the two legs from the repository root and diff the per-hop sha256 logs.
// The parser comparison spans these two commands, never two parsers linked
// into one binary; each binary exercises only the parser it was compiled
// with, so differing outputs separate a reader defect from corrupted bytes.
//
//	bound/es-lite leg:
//	go tool github.com/s4wave/goscript/cmd/goscript test --dir . --timeout 10m -p 1 \
//	  --tags skip_e2e,purego,goscript --protobuf-ts-binding \
//	  --run TestTransformStepConfigBindingChain ./core/sobject
//
//	native twin (protobuf-go-lite):
//	go test -count=1 -v -tags skip_e2e,purego,goscript \
//	  -run TestTransformStepConfigBindingChain ./core/sobject
//
// TestTransformStepConfigBindingChain walks the stored transform-config bytes
// through the same hops the first-mount path uses, hashing Steps[0].Config at
// each hop. A changed hash names the layer that mutates the bytes:
//
//	H0 marshal       (the MarshalVT implementation selected by this build;
//	                  the binding lowers it to es-lite toBinary under the
//	                  bound leg and keeps protobuf-go-lite natively)
//	H1 nested message  (SOGrantInner marshal -> unmarshal -> re-marshal)
//	H2 grant chain     (EncryptSOGrant -> SOState wire reopen -> DecryptInnerData)
//	H3 parse           (the build's UnmarshalVT over the captured bytes)
//
// The H2 SOState marshal/unmarshal covers the wire persistence boundary only.
// It does not write an actual KV store; a store-layer mutation would need a
// separate hop against the real store implementation.
func TestTransformStepConfigBindingChain(t *testing.T) {
	ctx := context.Background()

	// Deterministic key and object ID so both legs hash identical inputs.
	var encKey [32]byte
	for i := range encKey {
		encKey[i] = byte(i)
	}

	// H0: capture the bound marshal output for the leaf step config.
	encConf := &transform_blockenc.Config{
		BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
		Key:      bytes.Clone(encKey[:]),
	}
	conf := &block_transform.Config{
		Steps: []*block_transform.StepConfig{{
			Id:     transform_blockenc.ConfigID,
			Config: mustMarshalVT(t, encConf),
		}},
	}
	h0 := hashStepConfigBytes(t, "H0", conf.Steps[0].Config)

	// H1: round-trip the conf through the nested SOGrantInner message.
	inner := &SOGrantInner{TransformConf: conf}
	innerData := mustMarshalVT(t, inner)
	innerBack := &SOGrantInner{}
	if err := innerBack.UnmarshalVT(innerData); err != nil {
		t.Fatalf("H1 unmarshal SOGrantInner: %v", err)
	}
	if reData := mustMarshalVT(t, innerBack); !bytes.Equal(innerData, reData) {
		t.Fatalf("H1 SOGrantInner re-marshal differs: %d vs %d bytes", len(innerData), len(reData))
	}
	h1 := hashStepConfigBytes(t, "H1", innerBack.GetTransformConf().GetSteps()[0].Config)
	if !bytes.Equal(h0, h1) {
		t.Fatalf("H1 nested message changed step config bytes: H0 %x vs H1 %x", h0, h1)
	}

	// H2: encrypt the grant, cross the SOState wire boundary, decrypt back.
	peers := createMockPeers(t, 1)
	priv, err := peers[0].GetPrivKey(ctx)
	if err != nil {
		t.Fatalf("H2 get priv key: %v", err)
	}
	pub, err := peers[0].GetPeerID().ExtractPublicKey()
	if err != nil {
		t.Fatalf("H2 extract peer pub key: %v", err)
	}
	grant, err := EncryptSOGrant(priv, pub, mockSharedObjectID, inner)
	if err != nil {
		t.Fatalf("H2 EncryptSOGrant: %v", err)
	}
	stateData := mustMarshalVT(t, &SOState{RootGrants: []*SOGrant{grant}})
	stateBack := &SOState{}
	if err := stateBack.UnmarshalVT(stateData); err != nil {
		t.Fatalf("H2 unmarshal SOState: %v", err)
	}
	if len(stateBack.GetRootGrants()) != 1 {
		t.Fatalf("H2 expected 1 root grant, got %d", len(stateBack.GetRootGrants()))
	}
	innerDec, err := stateBack.GetRootGrants()[0].DecryptInnerData(priv, mockSharedObjectID)
	if err != nil {
		t.Fatalf("H2 DecryptInnerData: %v", err)
	}
	decSteps := innerDec.GetTransformConf().GetSteps()
	if len(decSteps) != 1 {
		t.Fatalf("H2 expected 1 transform step, got %d", len(decSteps))
	}
	h2 := hashStepConfigBytes(t, "H2", decSteps[0].Config)
	if !bytes.Equal(h0, h2) {
		t.Fatalf("H2 grant chain changed step config bytes: H0 %x vs H2 %x", h0, h2)
	}

	// H3: bound parse of the surviving bytes. Under the GoScript leg this
	// delegates to the es-lite fromBinary reader; under the native leg it is
	// the protobuf-go-lite parser. The twin command provides the other side.
	parsed := &transform_blockenc.Config{}
	if err := parsed.UnmarshalVT(decSteps[0].Config); err != nil {
		t.Fatalf("H3 bound parse of step config: %v", err)
	}
	if parsed.GetBlockEnc() != blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305 {
		t.Fatalf("H3 parsed block_enc %v, want XCHACHA20_POLY1305", parsed.GetBlockEnc())
	}
	if !bytes.Equal(parsed.GetKey(), encKey[:]) {
		t.Fatalf("H3 parsed key differs from input: %x", parsed.GetKey())
	}
	t.Logf("H3 bound parse ok; native-vs-bound comparison spans the two commands compiling this file")
}

// hashStepConfigBytes logs the sha256 and first 16 bytes of a captured step
// config and returns the digest for hop comparisons.
func hashStepConfigBytes(t *testing.T, hop string, data []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(data)
	preview := data
	if len(preview) > 16 {
		preview = preview[:16]
	}
	t.Logf("%s: len=%d sha256=%s head=%s", hop, len(data), hex.EncodeToString(sum[:]), hex.EncodeToString(preview))
	return sum[:]
}
