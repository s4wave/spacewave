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

// Equals compares to the other block.
func (t *testNamedSubBlock) Equals(ot ComparableNamedSubBlock) bool {
	ov, ok := ot.(*testNamedSubBlock)
	if !ok {
		return false
	}
	return ov == t
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

// TestCompareNamedSubBlocks tests comparing two sets of named sub blocks.
func TestCompareNamedSubBlocks(t *testing.T) {
	setA := make([]*testNamedSubBlock, 100)
	for i := range setA {
		setA[i] = &testNamedSubBlock{name: "foo-" + strconv.Itoa(i)}
	}

	setB := make([]*testNamedSubBlock, 100)
	copy(setB, setA)

	// modify setB a bit
	// note: we ignore nil values
	setB = setB[2:]
	setB[20] = &testNamedSubBlock{name: "bar"}
	setB[0] = &testNamedSubBlock{name: "foo-2"} // Equals() == false

	added, removed, changed := CompareNamedSubBlocks(setA, setB)
	if len(removed) != 3 || len(added) != 1 || len(changed) != 1 {
		t.Fail()
	}
}
