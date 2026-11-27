package forge_target

// IsNil checks if the object is nil.
func (i *Input) IsNil() bool {
	return i == nil
}

// Validate validates the Input object.
func (i *Input) Validate() error {
	// Accept the empty input, or require a supported input type.
	if i.GetInputType() == InputType_InputType_UNKNOWN {
		// assume empty
		return nil
	}

	if err := i.GetInputType().validate(false); err != nil {
		return err
	}

	// Validate the arguments belonging to the selected input type.
	switch i.GetInputType() {
	case InputType_InputType_VALUE:
		if err := i.GetValue().Validate(true); err != nil {
			return err
		}
	case InputType_InputType_WORLD:
		if err := i.GetWorld().validate(); err != nil {
			return err
		}
	case InputType_InputType_WORLD_OBJECT:
		if err := i.GetWorldObject().validate(); err != nil {
			return err
		}
	case InputType_InputType_TASK_OUTPUT:
		if err := i.GetTaskOutput().validate(); err != nil {
			return err
		}
	}
	return nil
}
