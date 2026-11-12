package space_world

// SetAdmitStep sets the hook Admit runs at each named point, and returns a
// function that restores the previous one.
func SetAdmitStep(fn func(step string) error) func() {
	prev := admitStep
	admitStep = fn
	return func() { admitStep = prev }
}
