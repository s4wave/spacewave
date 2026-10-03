package block

import (
	"math/rand/v2"
	"strconv"
	"testing"
)

// testNamedSubBlock is an example NamedSubBlock.
type testNamedSubBlock struct {
	alias AliasIdentityToken
	name  string
}

// GetName returns the name of the block.
func (t *testNamedSubBlock) GetName() string {
	return t.name
}

// IsNil returns if the object is nil.
func (t *testNamedSubBlock) IsNil() bool {
	return t == nil
}

// BlockAliasIdentity returns the in-memory alias token for testNamedSubBlock.
func (t *testNamedSubBlock) BlockAliasIdentity() *AliasIdentityToken {
	return &t.alias
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
var _ NamedSubBlock = (*testNamedSubBlock)(nil)

// TestSortNamedSubBlocks tests sorting a set of named sub blocks.
func TestSortNamedTestBlocks(t *testing.T) {
	// Generate the original names for named sub-block sorting.
	namesSorted := make([]string, 100)
	for i := range namesSorted {
		namesSorted[i] = "foo-" + strconv.Itoa(i)
	}

	// Prepare and shuffle the input name slots.
	namesShuffled := make([]string, len(namesSorted))
	rand.Shuffle(len(namesShuffled), func(i, j int) {
		namesShuffled[i], namesShuffled[j] = namesShuffled[j], namesShuffled[i]
	})

	// Construct the named sub-blocks from shuffled names.
	namedSubBlocks := make([]*testNamedSubBlock, len(namesShuffled))
	for i := range namedSubBlocks {
		namedSubBlocks[i] = &testNamedSubBlock{name: namesShuffled[i]}
	}

	// Sort the named sub-blocks and verify their order.
	SortNamedSubBlocks(namedSubBlocks)
	if !IsNamedSubBlocksSorted(namedSubBlocks) {
		t.Fail()
	}
}

// TestCompareNamedSubBlocks tests comparing two sets of named sub blocks.
func TestCompareNamedSubBlocks(t *testing.T) {
	// Construct the original named sub-block set.
	setA := make([]*testNamedSubBlock, 100)
	for i := range setA {
		setA[i] = &testNamedSubBlock{name: "foo-" + strconv.Itoa(i)}
	}

	// Copy the original set for change comparisons.
	setB := make([]*testNamedSubBlock, 100)
	copy(setB, setA)

	// modify setB a bit
	// note: we ignore nil values
	setB = setB[2:]
	setB[20] = &testNamedSubBlock{name: "bar"}
	setB[0] = &testNamedSubBlock{name: "foo-2"} // Equals() == false

	// Compare the modified set and verify addition, removal, and change counts.
	added, removed, changed := CompareNamedSubBlocks(setA, setB)
	if len(removed) != 3 || len(added) != 1 || len(changed) != 1 {
		t.Fail()
	}
}
