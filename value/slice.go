package forge_value

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// ValueSlice is a slice of values.
type ValueSlice []*Value

// Clone clones the slice of values.
func (v ValueSlice) Clone() ValueSlice {
	out := make(ValueSlice, len(v))
	for i := range out {
		out[i] = v[i].Clone()
	}
	return out
}

// RemoveUnknown filters any values with an empty name or type.
func (v ValueSlice) RemoveUnknown() ValueSlice {
	for i := 0; i < len(v); i++ {
		outp := v[i]
		if outp.GetValueType() == 0 || outp.GetName() == "" {
			v[i] = v[len(v)-1]
			v[len(v)-1] = nil
			v = v[:len(v)-1]
			i--
		}
	}
	return v
}

// Validate checks all values in the slice.
func (v ValueSlice) Validate(allowEmptyName, checkDupe, checkSort bool) error {
	m := make(map[string]int)
	for i, val := range v {
		if err := val.Validate(allowEmptyName); err != nil {
			return errors.Wrapf(err, "values[%d]", i)
		}
		if checkDupe {
			name := val.GetName()
			if oi, ok := m[name]; ok {
				return errors.Wrapf(
					errors.Wrapf(ErrDuplicateValueName, "%q", name),
					"values[%d] and values[%d]", oi, i,
				)
			}
			m[name] = i
		}
	}
	if checkSort && !v.IsSorted() {
		return errors.New("values: must be sorted by name")
	}
	return nil
}

// BuildValueMap constructs a ValueMap from the ValueSlice.
func (v ValueSlice) BuildValueMap(checkDupe, cloneObjs bool) (ValueMap, error) {
	m := make(map[string]*Value)
	for _, val := range v {
		valName := val.GetName()
		if valName == "" {
			return nil, ErrEmptyValueName
		}
		if checkDupe {
			if _, ok := m[valName]; ok {
				return nil, errors.Wrap(ErrDuplicateValueName, valName)
			}
		}
		if cloneObjs {
			val = val.Clone()
		}
		m[valName] = val
	}
	return m, nil
}

// SortByName sorts the value slice by name.
func (v ValueSlice) SortByName() {
	block.SortNamedSubBlocks(v)
}

// IsSorted checks if the value slice is sorted.
func (v ValueSlice) IsSorted() bool {
	return block.IsNamedSubBlocksSorted(v)
}

// Merge applies a second set of values while overwriting existing.
// Note: we do not expect large value pointer sets.
// Any large data-set should be held under a block DAG structure.
func (v ValueSlice) Merge(vals ValueSlice) ValueSlice {
	var doSort bool
	m := make(map[string]int)
	for exi, ex := range v {
		m[ex.GetName()] = exi
	}
	for _, val := range vals {
		// replace existing matching
		valName := val.GetName()
		exi, exOk := m[valName]
		if exOk {
			v[exi] = val
		}
		// append otherwise
		m[valName] = len(v)
		v = append(v, val)
		doSort = true
	}
	if doSort {
		v.SortByName()
	}
	return v
}

// Equals compares two sets of values for equality.
func (v ValueSlice) Equals(ot ValueSlice) bool {
	if len(v) != len(ot) {
		return false
	}
	if !v.IsSorted() || !ot.IsSorted() {
		added, removed, changed := v.Compare(ot)
		return len(added)+len(removed)+len(changed) == 0
	}
	for i := 0; i < len(ot); i++ {
		if !v[i].Equals(v[i]) {
			return false
		}
	}
	return true
}

// Compare returns the added, removed, and changed values.
func (v ValueSlice) Compare(b ValueSlice) (added []*Value, removed []*Value, changed []*Value) {
	return block.CompareNamedSubBlocks(v, b)
}
