//go:build !js

package bldr_project_starlark

import "github.com/pkg/errors"

func errNoPositionalArgs(fn string) error {
	return errors.Errorf("%s() does not accept positional arguments", fn)
}

func errExpectedString(fn, field string) error {
	return errors.Errorf("%s(): %s must be a string", fn, field)
}

func errExpectedBool(fn, field string) error {
	return errors.Errorf("%s(): %s must be a bool", fn, field)
}

func errExpectedList(fn, field string) error {
	return errors.Errorf("%s(): %s must be a list", fn, field)
}

func errExpectedDict(fn, field string) error {
	return errors.Errorf("%s(): %s must be a dict", fn, field)
}

func errExpectedStringInList(fn, field string) error {
	return errors.Errorf("%s(): %s must contain only strings", fn, field)
}

func errExpectedInt(fn, field string) error {
	return errors.Errorf("%s(): %s must be an int", fn, field)
}

func errUnknownKwarg(fn, key string) error {
	return errors.Errorf("%s(): unknown keyword argument %q", fn, key)
}

func errUnknownField(parent, key string) error {
	return errors.Errorf("%s: unknown field %q", parent, key)
}
