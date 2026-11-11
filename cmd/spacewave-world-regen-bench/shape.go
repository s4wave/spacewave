//go:build !js && !wasip1

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// A result is shaped like a Hyperline course result: an object with a body of
// resultBodyBytes and three quads, to its type, the player it scores and the
// host that ran it. The players and hosts are hubs of many incoming edges.
const (
	resultBodyBytes = 512
	resultPlayers   = 50_000
	resultHosts     = 64
	resultTypeKey   = "t/result"
	resultTypePred  = "<hl/type>"
	resultPlayer    = "<hl/player>"
	resultHost      = "<hl/host>"
	resultPrefix    = "r/"
)

// resultKey is the object key of result i, sorting in i's order.
func resultKey(i int) string {
	return fmt.Sprintf("%s%010d", resultPrefix, i)
}

// playerKey and hostKey name the hubs.
func playerKey(i int) string { return fmt.Sprintf("p/%05d", i) }
func hostKey(i int) string   { return fmt.Sprintf("h/%02d", i) }

// hubKeys lists every hub object key.
func hubKeys() []string {
	keys := []string{resultTypeKey}
	for i := range resultPlayers {
		keys = append(keys, playerKey(i))
	}
	for i := range resultHosts {
		keys = append(keys, hostKey(i))
	}
	return keys
}

// resultQuads are result i's three quads.
func resultQuads(i int) [3]world.GraphQuad {
	key := resultKey(i)
	return [3]world.GraphQuad{
		world.NewGraphQuadWithKeys(key, resultTypePred, resultTypeKey, ""),
		world.NewGraphQuadWithKeys(key, resultPlayer, playerKey(i*7919%resultPlayers), ""),
		world.NewGraphQuadWithKeys(key, resultHost, hostKey(i%resultHosts), ""),
	}
}

// resultBody is result i's body text.
func resultBody(i int) string {
	head := fmt.Sprintf("result %d ", i)
	return head + strings.Repeat("x", resultBodyBytes-len(head))
}

// writeHubs creates the hub objects, or leaves those that exist.
func writeHubs(ctx context.Context, ws world.WorldState) error {
	for _, key := range hubKeys() {
		if _, err := putObject(ctx, ws, key, key); err != nil {
			return err
		}
	}
	return nil
}

// writeResults writes results [start, end) with their bodies and quads the
// way Hyperline writes course results: inside one World operation, holding the
// quads of every result it creates for one graph index update. A result
// written before is written again unchanged, so a commit a kill left
// unacknowledged can be retried; its quads exist already, so it sets them one
// at a time, which keeps them.
func writeResults(ctx context.Context, ws world.WorldState, start, end int) error {
	_, _, err := ws.ApplyWorldOp(ctx, &blockWorldOp{fn: func(ctx context.Context, bws *world_block.WorldState) error {
		quads := make([]world.GraphQuad, 0, 3*(end-start))
		for i := start; i < end; i++ {
			created, err := putObject(ctx, bws, resultKey(i), resultBody(i))
			if err != nil {
				return err
			}
			rq := resultQuads(i)
			if created {
				quads = append(quads, rq[:]...)
				continue
			}
			for _, q := range rq {
				if err := bws.SetGraphQuad(ctx, q); err != nil {
					return err
				}
			}
		}
		return bws.InsertGraphQuads(ctx, quads)
	}}, "")
	return err
}

// putObject writes body as a block and creates or points key at it. It
// reports whether it created the object.
func putObject(ctx context.Context, ws world.WorldState, key, body string) (bool, error) {
	// Write the body through the World's storage.
	c, err := ws.BuildStorageCursor(ctx)
	if err != nil {
		return false, err
	}
	defer c.Release()

	// Write the body block and reference it in the World's bucket.
	btx, bcs := c.BuildTransaction(nil)
	bcs.SetBlock(block_mock.NewExample(body), true)
	ref, _, err := btx.Write(ctx, true)
	if err != nil {
		return false, err
	}
	next := c.GetRef().Clone()
	next.RootRef = ref

	// Point an existing object at it, or create the object.
	obj, found, err := ws.GetObject(ctx, key)
	if err != nil || found {
		if err == nil {
			_, err = obj.SetRootRef(ctx, next)
		}
		world.ReleaseObjectState(obj)
		return false, err
	}
	created, err := ws.CreateObject(ctx, key, next)
	world.ReleaseObjectState(created)
	return err == nil, err
}

// blockWorldOp runs fn on the block World under a transaction, as a Hyperline
// World operation runs in-process. It is never encoded.
type blockWorldOp struct {
	fn func(ctx context.Context, ws *world_block.WorldState) error
}

// TypeID names the operation.
func (*blockWorldOp) TypeID() string { return "spacewave-world-regen-bench/block" }

// MarshalVT refuses: the operation is never encoded.
func (*blockWorldOp) MarshalVT() ([]byte, error) { return nil, errOpNotEncoded }

// UnmarshalVT refuses: the operation is never encoded.
func (*blockWorldOp) UnmarshalVT([]byte) error { return errOpNotEncoded }

// ApplyWorldOp hands the block World to fn.
func (o *blockWorldOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, _ peer.ID) (bool, error) {
	bws, ok := ws.(*world_block.WorldState)
	if !ok {
		return false, errors.Errorf("bench op needs a block World, got %T", ws)
	}
	return false, o.fn(ctx, bws)
}

// errOpNotEncoded is returned by blockWorldOp's codec.
var errOpNotEncoded = errors.New("bench op is not encoded")

// commitTx runs fn in one write transaction on w and commits it.
func commitTx(ctx context.Context, w *benchWorld, fn func(ws world.WorldState) error) error {
	// Open the transaction, apply fn and commit.
	tx, err := w.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
