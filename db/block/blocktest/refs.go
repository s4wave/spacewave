// Package blocktest holds checks for tests of block reference handling.
package blocktest

import (
	"reflect"
	"slices"
	"strconv"

	"github.com/s4wave/spacewave/db/block"
)

// refType is the type of a block reference pointer.
var refType = reflect.TypeFor[*block.BlockRef]()

// UnsetRefs returns the path of every block reference msg can hold that is
// unset: an empty reference, a nil message that can hold one, or an empty
// repeated field or map whose elements can hold one. It walks exported fields
// to any depth, so a test fixture that must name every root its type carries
// fails when the type gains a reference the fixture does not set.
func UnsetRefs(msg any) []string {
	var w refWalker
	w.walk(reflect.ValueOf(msg), reflect.TypeOf(msg).String())
	return w.unset
}

// refWalker collects unset reference paths.
type refWalker struct {
	unset []string
}

// walk records the unset references under v, found at path.
func (w *refWalker) walk(v reflect.Value, path string) {
	t := v.Type()
	if !holdsRef(t, nil) {
		return
	}
	switch {
	case t == refType:
		if v.Interface().(*block.BlockRef).GetEmpty() {
			w.unset = append(w.unset, path)
		}
	case t.Kind() == reflect.Pointer:
		if v.IsNil() {
			w.unset = append(w.unset, path)
			return
		}
		w.walk(v.Elem(), path)
	case t.Kind() == reflect.Struct:
		for i := range t.NumField() {
			if f := t.Field(i); f.IsExported() {
				w.walk(v.Field(i), path+"."+f.Name)
			}
		}
	case t.Kind() == reflect.Slice:
		if v.Len() == 0 {
			w.unset = append(w.unset, path)
		}
		for i := range v.Len() {
			w.walk(v.Index(i), path+"["+strconv.Itoa(i)+"]")
		}
	case t.Kind() == reflect.Map:
		if v.Len() == 0 {
			w.unset = append(w.unset, path)
		}
		iter := v.MapRange()
		for iter.Next() {
			w.walk(iter.Value(), path+"[key]")
		}
	}
}

// holdsRef reports whether a value of type t can hold a block reference,
// skipping the types in visiting, which a recursive message reaches again.
func holdsRef(t reflect.Type, visiting []reflect.Type) bool {
	// A block reference holds itself; a type already on the path adds nothing.
	if t == refType {
		return true
	}
	if slices.Contains(visiting, t) {
		return false
	}
	visiting = append(visiting, t)
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return holdsRef(t.Elem(), visiting)
	case reflect.Struct:
		for f := range t.Fields() {
			if f.IsExported() && holdsRef(f.Type, visiting) {
				return true
			}
		}
	}
	return false
}
