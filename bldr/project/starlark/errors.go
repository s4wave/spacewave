//go:build !js

package bldr_project_starlark

import "github.com/pkg/errors"

// errNoPositionalArgs builds the error for unexpected positional arguments.
func errNoPositionalArgs(fn string) error {
	return errors.Errorf("%s() does not accept positional arguments", fn)
}

// errExpectedString builds the error for a non-string field value.
func errExpectedString(fn, field string) error {
	return errors.Errorf("%s(): %s must be a string", fn, field)
}

// errExpectedBool builds the error for a non-bool field value.
func errExpectedBool(fn, field string) error {
	return errors.Errorf("%s(): %s must be a bool", fn, field)
}

// errExpectedList builds the error for a non-list field value.
func errExpectedList(fn, field string) error {
	return errors.Errorf("%s(): %s must be a list", fn, field)
}

// errExpectedDict builds the error for a non-dict field value.
func errExpectedDict(fn, field string) error {
	return errors.Errorf("%s(): %s must be a dict", fn, field)
}

// errExpectedStringInList builds the error for a list containing
// non-string entries.
func errExpectedStringInList(fn, field string) error {
	return errors.Errorf("%s(): %s must contain only strings", fn, field)
}

// errExpectedInt builds the error for a non-int field value.
func errExpectedInt(fn, field string) error {
	return errors.Errorf("%s(): %s must be an int", fn, field)
}

// errUnknownKwarg builds the error for an unrecognized keyword argument.
func errUnknownKwarg(fn, key string) error {
	return errors.Errorf("%s(): unknown keyword argument %q", fn, key)
}

// errUnknownField builds the error for an unrecognized field in a decoded
// parent object.
func errUnknownField(parent, key string) error {
	return errors.Errorf("%s: unknown field %q", parent, key)
}
