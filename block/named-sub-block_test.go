package block

import (
	"math/rand"
	"strconv"
	"testing"
)

// testNamedSubBlock is an example NamedSubBlock.
type testNamedSubBlock struct {
	name string
}

// GetName returns the name of the block.
func (t *testNamedSubBlock) GetName() string {
	return t.name
}

// _ is a type assertion
var _ NamedSubBlock = ((*testNamedSubBlock)(nil))

// TestSortNamedSubBlocks tests sorting a set of named sub blocks.
func TestSortNamedTestBlocks(t *testing.T) {
	namesSorted := make([]string, 100)
	for i := 0; i < len(namesSorted); i++ {
		namesSorted[i] = "foo-" + strconv.Itoa(i)
	}

	namesShuffled := make([]string, len(namesSorted))
	rand.Shuffle(len(namesShuffled), func(i, j int) {
		namesShuffled[i], namesShuffled[j] = namesShuffled[j], namesShuffled[i]
	})

	namedSubBlocks := make([]*testNamedSubBlock, len(namesShuffled))
	for i := range namedSubBlocks {
		namedSubBlocks[i] = &testNamedSubBlock{name: namesShuffled[i]}
	}

	SortNamedSubBlocks(namedSubBlocks)
	if !IsNamedSubBlocksSorted(namedSubBlocks) {
		t.Fail()
	}
}
