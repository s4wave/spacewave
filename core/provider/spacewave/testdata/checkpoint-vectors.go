//go:build ignore

// Regenerate with go run -tags purego ./core/provider/spacewave/testdata/checkpoint-vectors.go.
// The fixed key is public test material and must never identify a real account.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

func main() {
	// Derive the fixed author key.
	const objectID = "checkpoint-protocol"
	key, _, err := crypto.GenerateEd25519Key(bytes.NewReader(bytes.Repeat([]byte{42}, 32)))
	must(err)
	peerID, err := peer.IDFromPrivateKey(key)
	must(err)

	// Sign one author chain of 50 operations under a fixed config hash.
	configHash := sha256.Sum256([]byte("checkpoint config"))
	var operations []*sobject.SOOperation
	var prev []byte
	for nonce := uint64(1); nonce <= 50; nonce++ {
		link := &sobject.SOOperationLink{Nonce: nonce, PrevOpHash: prev, ConfigHash: configHash[:]}
		operation, err := sobject.BuildSOOperation(objectID, key, []byte(fmt.Sprintf("edit %d", nonce)), link, fmt.Sprintf("01j0000000%016d", nonce))
		must(err)
		operations = append(operations, operation)
		prev = sobject.HashSOOperationInner(operation.GetInner())
	}
	batch, err := (&api.PostOpsRequest{Operations: operations}).MarshalVT()
	must(err)

	// Sign a root that resolves the chain head and rejects its last operation.
	rejection, err := sobject.BuildSOOperationRejection(key, objectID, peerID, 50, fmt.Sprintf("01j0000000%016d", 50), nil)
	must(err)
	inner, err := (&sobject.SORootInner{Seqno: 51, StateData: []byte("coalesced checkpoint")}).MarshalVT()
	must(err)
	root := &sobject.SORoot{Inner: inner, InnerSeqno: 51, AccountNonces: []*sobject.SOAccountNonce{{PeerId: peerID.String(), Nonce: 50, OpHash: prev}}}
	must(root.SignInnerData(key, objectID, 51, hash.RecommendedHashType))
	request, err := (&api.PostRootRequest{Root: root, RejectedOps: []*sobject.SOOperationRejection{rejection}}).MarshalVT()
	must(err)

	// Write the vectors as JSON.
	must(json.NewEncoder(os.Stdout).Encode(struct {
		ObjectID string `json:"objectId"`
		PeerID   string `json:"peerId"`
		Batch    []byte `json:"batch"`
		Root     []byte `json:"root"`
	}{objectID, peerID.String(), batch, request}))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
