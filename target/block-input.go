package forge_target

// NewInput_World constructs a new Input for a World.
func NewInput_World(engineID string) *Input {
	return &Input{
		InputType: InputType_InputType_WORLD,
		World: &InputWorld{
			EngineId: engineID,
		},
	}
}

// Validate validates the Input object.
func (i *Input) Validate() error {
	if i.GetInputType() == InputType_InputType_UNKNOWN {
		// assume empty
		return nil
	}

	if err := i.GetInputType().Validate(false); err != nil {
		return err
	}
	switch i.GetInputType() {
	case InputType_InputType_VALUE:
		if err := i.GetValue().Validate(true); err != nil {
			return err
		}
	case InputType_InputType_WORLD:
		if err := i.GetWorld().Validate(); err != nil {
			return err
		}
	case InputType_InputType_WORLD_OBJECT:
		if err := i.GetWorldObject().Validate(); err != nil {
			return err
		}
	}
	return nil
}
