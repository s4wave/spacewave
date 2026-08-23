package opfsbrowserharness

import (
	"context"
	"fmt"

	"github.com/s4wave/spacewave/db/bucket"
	world "github.com/s4wave/spacewave/db/world"
)

// CheckWorld writes objects and a graph edge and reads them back.
func CheckWorld(ctx context.Context, ws world.WorldState) error {
	objRef := &bucket.ObjectRef{BucketId: "test-bucket"}
	for _, key := range []string{"task/a", "task/b"} {
		if _, err := ws.CreateObject(ctx, key, objRef); err != nil {
			return fmt.Errorf("create object %s: %w", key, err)
		}
	}

	quad := world.NewGraphQuadWithKeys("task/a", "<assigned-to>", "task/b", "")
	if err := ws.SetGraphQuad(ctx, quad); err != nil {
		return fmt.Errorf("set graph quad: %w", err)
	}

	// Read back through the same WorldState API a TypeScript SDK would expose.
	obj, ok, err := ws.GetObject(ctx, "task/a")
	if err != nil {
		return fmt.Errorf("get object task/a: %w", err)
	}
	if !ok {
		return fmt.Errorf("object task/a not found after create")
	}
	rootRef, _, err := obj.GetRootRef(ctx)
	world.ReleaseObjectState(obj)
	if err != nil {
		return fmt.Errorf("read root ref task/a: %w", err)
	}
	if rootRef == nil {
		return fmt.Errorf("object task/a returned nil root ref")
	}

	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys("task/a", "<assigned-to>", "", ""),
		0,
	)
	if err != nil {
		return fmt.Errorf("lookup graph quads: %w", err)
	}
	if len(quads) != 1 || quads[0].GetObj() != world.KeyToGraphValue("task/b").String() {
		return fmt.Errorf("graph quads = %#v, want one edge to task/b", quads)
	}

	var iterKeys []string
	it := ws.IterateObjects(ctx, "task/", false)
	for it.Next() {
		iterKeys = append(iterKeys, it.Key())
	}
	// Read Err before Close: Close marks the iterator consumed.
	iterErr := it.Err()
	it.Close()
	if iterErr != nil {
		return fmt.Errorf("iterate objects: %w", iterErr)
	}
	if len(iterKeys) != 2 {
		return fmt.Errorf("iterated keys = %#v, want task/a and task/b", iterKeys)
	}

	seqno, err := ws.GetSeqno(ctx)
	if err != nil {
		return fmt.Errorf("get seqno: %w", err)
	}

	fmt.Printf(
		"opfs-browser-harness OK objects=%v edge=task/a<assigned-to>task/b seqno=%d\n",
		iterKeys, seqno,
	)
	return nil
}
