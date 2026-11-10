//go:build !js && !wasip1

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// checkArgs prove the head holds exactly the hubs and results [0, n) for an
// n at least the last acknowledged commit's end, each result with its three
// quads and nothing else. A commit a kill left unacknowledged may have landed
// and counts toward n.
type checkArgs struct {
	// Path is the database file.
	Path string
}

// BuildFlags binds the flags.
func (a *checkArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "path", Usage: "database file", Destination: &a.Path, Required: true},
	}
}

// Run streams the head's objects and quads once each.
func (a *checkArgs) Run(c *cli.Context) error {
	// Open the World and read what was acknowledged.
	ctx := c.Context
	w, err := openBenchWorld(ctx, a.Path)
	if err != nil {
		return err
	}
	defer w.Close()
	seeded, err := lastInt(seededPath(a.Path))
	if err != nil {
		return err
	}
	acked, err := lastInt(ackedPath(a.Path))
	if err != nil {
		return err
	}

	// Read the stage and open a read of the head.
	stage, err := world_block.NewRegenerator(w.authority, world_block.RegenConfig{}).Stage(ctx)
	if err != nil {
		return err
	}
	tx, err := w.engine.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// The objects are the hubs and results in a run from zero.
	results, err := checkObjects(ctx, tx)
	if err != nil {
		return err
	}
	if want := max(seeded, acked); results < want {
		return errors.Errorf("head holds %d results, %d acknowledged", results, want)
	}

	// Each result has its three quads, and nothing else is in the graph.
	quads, err := checkQuads(ctx, tx, results)
	if err != nil {
		return err
	}
	if quads != 3*results {
		return errors.Errorf("head holds %d quads for %d results", quads, results)
	}
	fmt.Printf("ok: stage %v, %d results (%d acknowledged, %d seeded), %d quads\n", stage, results, acked, seeded, quads)
	return nil
}

// checkObjects streams the object keys, requiring every hub and the results
// [0, n) in order, and returns n.
func checkObjects(ctx context.Context, ws world.WorldState) (int, error) {
	// Walk the keys in order.
	hubs := make(map[string]bool)
	for _, key := range hubKeys() {
		hubs[key] = false
	}
	results := 0
	it := ws.IterateObjects(ctx, "", false)
	defer it.Close()
	for it.Next() {
		key := it.Key()
		if seen, ok := hubs[key]; ok && !seen {
			hubs[key] = true
			continue
		}
		if key != resultKey(results) {
			return 0, errors.Errorf("object %q where result %d belongs", key, results)
		}
		results++
	}
	if err := it.Err(); err != nil {
		return 0, err
	}

	// Every hub must be there.
	for key, seen := range hubs {
		if !seen {
			return 0, errors.Errorf("hub %q missing", key)
		}
	}
	return results, nil
}

// checkQuads streams every quad, requiring each to be one of a result's
// three, and returns the count. The graph is a set, so the count equal to
// three a result proves each result holds all three.
func checkQuads(ctx context.Context, ws world.WorldState, results int) (int, error) {
	n := 0
	err := ws.AccessCayleyGraph(ctx, false, func(ctx context.Context, h world.CayleyHandle) error {
		return world.IterateFilteredFullQuads(ctx, h, quad.Quad{}, func(q quad.Quad) error {
			// Find the result the quad's subject names.
			n++
			subject, _ := q.Subject.(quad.IRI)
			id, err := strconv.Atoi(strings.TrimPrefix(string(subject), resultPrefix))
			if err != nil || !strings.HasPrefix(string(subject), resultPrefix) || id >= results {
				return errors.Errorf("quad %v names no result", q)
			}

			// It must be one of that result's quads.
			for _, gq := range resultQuads(id) {
				want, err := world.GraphQuadToCayleyQuad(gq, false)
				if err != nil {
					return err
				}
				if want.String() == q.String() {
					return nil
				}
			}
			return errors.Errorf("quad %v is not one of result %d's", q, id)
		})
	})
	return n, err
}
