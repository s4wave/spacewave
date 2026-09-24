//go:build js

package compose

// Composition is the project-supplied part of a browser host process. Browser
// hosts have no command line, so the composition carries only factories.
type Composition struct {
	// Factories construct the project's controller factories on each host bus.
	Factories []AddFactoriesFunc
}
