package dot

import (
	"bytes"
	"context"
	"fmt"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/traverse"
)

// Plot plots a block structure, traversing references.
// visitorCb can be nil
func Plot(
	ctx context.Context,
	blk block.Block,
	btx *block.Transaction,
	bcs *block.Cursor,
	visitorCb traverse.Visitor,
) ([]byte, error) {
	// Fill the btx with the contents of the traversal.
	err := traverse.Visit(
		ctx,
		blk, bcs,
		func(loc *traverse.Location) error {
			if visitorCb != nil {
				if err := visitorCb(loc); err != nil {
					return err
				}
			}

			return nil
		},
		false,
	)
	if err != nil {
		return nil, err
	}

	return Marshal(btx.GetBlockGraph(), bcs.GetRef().MarshalString())
}

// Marshal renders the block graph in the Graphviz DOT language.
func Marshal(g *block.BlockGraph, name string) ([]byte, error) {
	out := &bytes.Buffer{}
	fmt.Fprintf(out, "digraph %q {\n", name)
	fmt.Fprintf(out, "\tgraph [rankdir=LR]\n")

	nodeIDs := make(map[int64]string)
	for _, nod := range g.Nodes() {
		id, err := dotNodeID(nod)
		if err != nil {
			return nil, err
		}
		nodeIDs[nod.ID()] = id
		line := fmt.Sprintf("\t%s", id)
		var attrs string
		if an, ok := nod.(block.AttributedNode); ok {
			attrs = dotAttrs(an.Attributes())
		}
		if attrs != "" {
			line += " [" + attrs + "]"
		}
		out.WriteString(line + "\n")
	}

	for _, e := range g.Edges() {
		fromID, ok := nodeIDs[e.From().ID()]
		if !ok {
			continue
		}
		toID, ok := nodeIDs[e.To().ID()]
		if !ok {
			continue
		}
		fmt.Fprintf(out, "\t%s -> %s\n", fromID, toID)
	}

	out.WriteString("}\n")
	return out.Bytes(), nil
}

// dotNodeID returns the quoted DOT identifier for a node.
func dotNodeID(nod block.GraphNode) (string, error) {
	if an, ok := nod.(block.AttributedNode); ok {
		dotID := an.DOTID()
		if dotID != "" {
			return fmt.Sprintf("%q", dotID), nil
		}
	}
	return "", fmt.Errorf("node %d has no DOT identifier", nod.ID())
}

// dotAttrs renders graph attributes as DOT key-value pairs.
func dotAttrs(attrs []block.BlockGraphAttribute) string {
	out := ""
	for _, attr := range attrs {
		if out != "" {
			out += " "
		}
		out += fmt.Sprintf("%s=%q", attr.Key, attr.Value)
	}
	return out
}
