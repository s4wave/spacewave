package forge_target

// GetInputsNames returns the list of names for a set of inputs.
func GetInputsNames(inps []*Input) []string {
	out := make([]string, len(inps))
	for i, inp := range inps {
		out[i] = inp.GetName()
	}
	return out
}
