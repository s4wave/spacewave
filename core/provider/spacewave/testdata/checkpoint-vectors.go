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

	// Sign the genesis checkpoint and two successors: one covering the first
	// half of the chain and one covering all of it.
	genesis, err := sobject.BuildGenesisSOCheckpoint(key, objectID, configHash[:], []byte("genesis"))
	must(err)
	cover := func(nonce uint64) []byte {
		head := sobject.HashSOOperationInner(operations[nonce-1].GetInner())
		checkpoint, err := sobject.BuildSOCheckpoint(key, &sobject.SOCheckpointInner{
			SharedObjectId:     objectID,
			Height:             1,
			PrevCheckpointHash: genesis.Hash(),
			ConfigHash:         configHash[:],
			StateData:          []byte(fmt.Sprintf("state at %d", nonce)),
			ReplayVersion:      sobject.SOReplayVersion,
			Authors:            []*sobject.SOOperationPosition{{PeerId: peerID.String(), Nonce: nonce, OpHash: head}},
		})
		must(err)
		return request(checkpoint)
	}

	// Write the vectors as JSON.
	must(json.NewEncoder(os.Stdout).Encode(struct {
		ObjectID   string `json:"objectId"`
		PeerID     string `json:"peerId"`
		ConfigHash []byte `json:"configHash"`
		Batch      []byte `json:"batch"`
		Genesis    []byte `json:"genesis"`
		Partial    []byte `json:"partial"`
		Checkpoint []byte `json:"checkpoint"`
	}{objectID, peerID.String(), configHash[:], batch, request(genesis), cover(25), cover(50)}))
}

// request encodes checkpoint as the body of POST /sobject/:id/checkpoint.
func request(checkpoint *sobject.SOCheckpoint) []byte {
	data, err := (&api.PostCheckpointRequest{Checkpoint: checkpoint}).MarshalVT()
	must(err)
	return data
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
