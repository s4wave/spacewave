package forge_value

import (
	"sort"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// ValueSlice is a slice of values.
type ValueSlice []*Value

// Validate checks all values in the slice.
func (v ValueSlice) Validate(allowEmptyName, checkDupe bool) error {
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
			val = proto.Clone(val).(*Value)
		}
		m[valName] = val
	}
	return m, nil
}

// SortByName sorts the value slice by name.
func (v ValueSlice) SortByName() {
	sort.Slice(v, func(i, j int) bool {
		return v[i].GetName() < v[j].GetName()
	})
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
