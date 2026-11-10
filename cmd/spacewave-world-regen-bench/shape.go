//go:build !js && !wasip1

package main

import (
	"context"
	"fmt"
	"strings"

	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
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
		if err := putObject(ctx, ws, key, key); err != nil {
			return err
		}
	}
	return nil
}

// writeResults writes results [start, end) with their bodies and quads. A
// result written before is written again unchanged, so a commit a kill left
// unacknowledged can be retried.
func writeResults(ctx context.Context, ws world.WorldState, start, end int) error {
	for i := start; i < end; i++ {
		if err := putObject(ctx, ws, resultKey(i), resultBody(i)); err != nil {
			return err
		}
		for _, q := range resultQuads(i) {
			if err := ws.SetGraphQuad(ctx, q); err != nil {
				return err
			}
		}
	}
	return nil
}

// putObject writes body as a block and creates or points key at it.
func putObject(ctx context.Context, ws world.WorldState, key, body string) error {
	// Write the body through the World's storage.
	c, err := ws.BuildStorageCursor(ctx)
	if err != nil {
		return err
	}
	defer c.Release()

	// Write the body block and reference it in the World's bucket.
	btx, bcs := c.BuildTransaction(nil)
	bcs.SetBlock(block_mock.NewExample(body), true)
	ref, _, err := btx.Write(ctx, true)
	if err != nil {
		return err
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
		return err
	}
	created, err := ws.CreateObject(ctx, key, next)
	world.ReleaseObjectState(created)
	return err
}

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
