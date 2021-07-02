package forge_lib_kvtx

import (
	"github.com/pkg/errors"
)

// checkReservedName checks if the name is reserved.
func checkReservedName(name string) error {
	if name == inputNameStore || name == outputNameStore {
		return errors.Errorf("input name is reserved: %s", name)
	}
	return nil
}

// IsEmpty checks if the configuration is empty.
func (o *Op) IsEmpty() bool {
	var any bool
	for _, op := range o.GetOps() {
		if !op.IsEmpty() {
			any = true
			break
		}
	}
	if any {
		return true
	}

	return o.GetOpType() == OpType_OpType_NONE && len(o.GetOps()) == 0
}

// Validate validates the configuration.
// Note: recursively checks nested ops.
func (o *Op) Validate() error {
	return o.validateRecursive(false, false)
}

// validateRecursive recursively validates the tree.
func (o *Op) validateRecursive(ignoreInput, ignoreOutput bool) error {
	if o.IsEmpty() {
		return nil
	}

	var inputWasSet bool
	var outputWasSet bool

	// checkKey ensures the key is set.
	if keyInput := o.GetKeyInput(); len(keyInput) != 0 {
		if err := checkReservedName(keyInput); err != nil {
			return errors.Wrap(err, "key_input")
		}
	}
	anyKeySet := len(o.GetKeyInput()) != 0 || len(o.GetKey()) != 0

	// checkInput checks the value input.
	checkInput := func(allowEmpty bool) error {
		if ki := o.GetValueString(); len(ki) != 0 {
			return nil
		}
		if kip := o.GetValueInput(); len(kip) != 0 {
			return checkReservedName(kip)
		}
		if o.GetValue().IsEmpty() {
			if allowEmpty {
				return nil
			}
			return errors.New("input value must be set")
		}
		inputWasSet = true
		return o.GetValue().Validate(true)
	}

	// checkOutput checks the output field
	checkOutput := func(allowEmpty bool) error {
		outputEmpty := len(o.GetOutput()) == 0
		if !allowEmpty && outputEmpty {
			return errors.New("output must be set")
		}
		if !outputEmpty {
			outputWasSet = true
			if err := checkReservedName(o.GetOutput()); err != nil {
				return err
			}
		}
		return nil
	}

	opType := o.GetOpType()
	if anyKeySet {
		switch opType {
		case OpType_OpType_GET_EXISTS:
			fallthrough
		case OpType_OpType_GET:
			if err := checkOutput(ignoreOutput); err != nil {
				return err
			}
		case OpType_OpType_CHECK_EXISTS:
			break
		case OpType_OpType_CHECK_NOT_EXISTS:
			break
		case OpType_OpType_CHECK_BLOB:
			fallthrough
		case OpType_OpType_CHECK:
			if err := checkInput(ignoreInput); err != nil {
				return err
			}
			if err := checkOutput(true); err != nil {
				return err
			}
		case OpType_OpType_SET_BLOB:
			fallthrough
		case OpType_OpType_SET:
			if err := checkInput(ignoreInput); err != nil {
				return err
			}
			if err := checkOutput(true); err != nil {
				return err
			}
		case OpType_OpType_DELETE:
			if err := checkOutput(true); err != nil {
				return err
			}
		case OpType_OpType_NONE:
			break
		default:
			return errors.Wrap(ErrUnknownOpType, opType.String())
		}
	}

	for _, op := range o.GetOps() {
		err := op.validateRecursive(ignoreInput || inputWasSet, ignoreOutput || outputWasSet)
		if err != nil {
			return err
		}
	}

	return nil
}
