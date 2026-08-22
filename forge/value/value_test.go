package forge_value

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestValidateRejectsOutOfRangeValueType pins that Validate fails closed on a
// value type outside the known set.
func TestValidateRejectsOutOfRangeValueType(t *testing.T) {
	v := &Value{ValueType: ValueType(99)}
	if err := v.Validate(true); err == nil {
		t.Fatal("expected out-of-range value type to fail validation")
	}
}

// TestValidateAcceptsKnownValueTypes pins the accepted value type set,
// including world-object snapshots which carry no ref payload.
func TestValidateAcceptsKnownValueTypes(t *testing.T) {
	blockRefVal := NewValueWithBlockRef("a", &block.BlockRef{})
	if err := blockRefVal.Validate(false); err != nil {
		t.Fatalf("block-ref value should validate: %v", err)
	}

	snapshotVal := &Value{ValueType: ValueType_ValueType_WORLD_OBJECT_SNAPSHOT}
	if err := snapshotVal.Validate(true); err != nil {
		t.Fatalf("world-object-snapshot value should validate: %v", err)
	}

	// unknown means empty; nothing further to validate
	empty := &Value{}
	if err := empty.Validate(true); err != nil {
		t.Fatalf("unknown/empty value should validate: %v", err)
	}
}
