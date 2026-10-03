package sobject

import (
	"bytes"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/net/crypto"
)

// leanOrderSets is the number of random operation sets compared with the Lean
// order model.
const leanOrderSets = 400

// TestLeanOrderConformance checks that Order and StablePoint make the same
// placements as the Lean model in lean/Spacewave/SObject/Order.lean, on random
// operation sets with forks, missing operations, covered prefixes and links
// that misstate a position.
func TestLeanOrderConformance(t *testing.T) {
	// Sign with four fixed authors.
	oracle := leanOracle(t)
	var privs []crypto.PrivKey
	var ids []string
	for _, name := range []string{"order a", "order b", "order c", "order d"} {
		priv, id := vectorKey(t, name)
		privs = append(privs, priv)
		ids = append(ids, id)
	}

	// Compare one random set per seed.
	var cases []leanCase
	for seed := range uint64(leanOrderSets) {
		rng := rand.New(rand.NewPCG(seed, 0x0de7))
		cases = append(cases, orderCase(t, rng, privs, ids, "orderOperations seed "+strconv.FormatUint(seed, 10)))
	}
	checkLeanCases(t, oracle, cases)
}

// orderCase signs one random operation set and records Go's order and stable
// point for it.
func orderCase(t *testing.T, rng *rand.Rand, privs []crypto.PrivKey, ids []string, name string) leanCase {
	// Sign operations that extend or fork their author's chain and name
	// random earlier operations, a few at a misstated nonce.
	t.Helper()
	var signed []*SOOperation
	var positions []*SOOperationPosition
	chains := make([][]*SOOperationPosition, len(privs))
	for range 4 + rng.IntN(12) {
		author := rng.IntN(len(privs))
		link := &SOOperationLink{Nonce: 1, ConfigHash: bytes.Repeat([]byte{3}, 32)}
		if chain := chains[author]; len(chain) != 0 {
			prev := chain[len(chain)-1]
			if rng.IntN(6) == 0 {
				prev = chain[rng.IntN(len(chain))]
			}
			link.Nonce = prev.GetNonce() + 1
			link.PrevOpHash = prev.GetOpHash()
		}
		for _, pos := range positions {
			if rng.IntN(4) != 0 {
				continue
			}
			parent := pos.CloneVT()
			if rng.IntN(8) == 0 {
				parent.Nonce = 1 + uint64(rng.IntN(3))
			}
			link.Parents = append(link.Parents, parent)
		}
		op, err := BuildSOOperation(vectorObjectID, privs[author], []byte("data"), link, NewSOOperationLocalID())
		if err != nil {
			t.Fatal(err)
		}
		pos := &SOOperationPosition{PeerId: ids[author], Nonce: link.Nonce, OpHash: op.Hash()}
		signed = append(signed, op)
		positions = append(positions, pos)
		chains[author] = append(chains[author], pos)
	}

	// Cover a random operation of some authors, and hold most of the rest.
	var checkpoint []*SOOperationPosition
	for _, chain := range chains {
		if len(chain) != 0 && rng.IntN(2) == 0 {
			checkpoint = append(checkpoint, chain[rng.IntN(len(chain))])
		}
	}
	set := NewSOOperationSet(vectorObjectID, &SOCheckpointInner{Authors: checkpoint})
	for _, op := range signed {
		if rng.IntN(8) == 0 {
			continue
		}
		if _, err := set.Add(op); err != nil {
			t.Fatal(err)
		}
	}
	roster := slices.DeleteFunc(slices.Clone(ids), func(string) bool { return rng.IntN(3) == 0 })

	// Number each hash by its rank in byte order, which keeps the order.
	var sorted [][]byte
	for _, pos := range positions {
		sorted = append(sorted, pos.GetOpHash())
	}
	slices.SortFunc(sorted, bytes.Compare)
	p := &orderProjection{rank: make(map[string]int, len(sorted))}
	for i, h := range sorted {
		p.rank[string(h)] = i
	}

	// Project the held operations.
	a := &p.a
	ops := a.NewArray()
	for _, pos := range positions {
		inner := set.Get(pos.GetOpHash())
		if inner == nil {
			continue
		}
		v := p.position(pos)
		v.Set("parents", p.positions(inner.GetParents()))
		v.Set("prev", a.NewNull())
		if prev := inner.GetPrevOpHash(); len(prev) != 0 {
			v.Set("prev", p.position(&SOOperationPosition{PeerId: pos.GetPeerId(), Nonce: pos.GetNonce() - 1, OpHash: prev}))
		}
		ops.SetArrayItem(len(ops.GetArray()), v)
	}

	// Ask for their order under the checkpoint and the roster.
	rosterList := a.NewArray()
	for i, id := range roster {
		rosterList.SetArrayItem(i, a.NewString(id))
	}
	req := a.NewObject()
	req.Set("op", a.NewString("orderOperations"))
	req.Set("ops", ops)
	req.Set("checkpoint", p.positions(checkpoint))
	req.Set("roster", rosterList)

	// Record Go's order and stable point.
	result := a.NewObject()
	result.Set("order", p.hashes(set.Order()))
	result.Set("stable", p.hashes(set.StablePoint(roster)))
	return leanCase{name: name, request: req.MarshalTo(nil), ok: true, field: "result", value: result.MarshalTo(nil)}
}

// orderProjection writes positions and hashes as oracle JSON, numbering each
// hash by its rank in byte order.
type orderProjection struct {
	// a owns the projected JSON.
	a fastjson.Arena
	// rank numbers each hash of the case.
	rank map[string]int
}

// position projects one position.
func (p *orderProjection) position(pos *SOOperationPosition) *fastjson.Value {
	// Write the peer, the nonce and the hash rank.
	v := p.a.NewObject()
	v.Set("peer", p.a.NewString(pos.GetPeerId()))
	v.Set("nonce", p.a.NewNumberInt(int(pos.GetNonce())))
	v.Set("hash", p.a.NewNumberInt(p.rank[string(pos.GetOpHash())]))
	return v
}

// positions projects a list of positions.
func (p *orderProjection) positions(list []*SOOperationPosition) *fastjson.Value {
	// Write each position in order.
	arr := p.a.NewArray()
	for i, pos := range list {
		arr.SetArrayItem(i, p.position(pos))
	}
	return arr
}

// hashes projects a list of hashes.
func (p *orderProjection) hashes(list [][]byte) *fastjson.Value {
	// Write each hash rank in order.
	arr := p.a.NewArray()
	for i, h := range list {
		arr.SetArrayItem(i, p.a.NewNumberInt(p.rank[string(h)]))
	}
	return arr
}
