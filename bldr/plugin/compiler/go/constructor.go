//go:build !js

package bldr_plugin_compiler_go

import "github.com/pkg/errors"

// Constructor describes the callable shape of a package-scope declaration.
// Parameter and result types remain the generated wrapper compiler's responsibility.
type Constructor struct {
	// IsFunction reports the same signature classification as go/types.
	IsFunction bool
	// Parameters counts individual parameters, including a variadic tail.
	Parameters int
	// Variadic indicates that the final parameter is optional at call sites.
	Variadic bool
}

// NeedsBus validates NewFactory's required arity and reports whether to pass the bus.
func (c *Constructor) NeedsBus(pkgPath string) (bool, error) {
	// Reject non-signature objects before checking their parameter count.
	if !c.IsFunction {
		return false, errors.Errorf("package %s NewFactory is not a function", pkgPath)
	}

	// Generated factory calls omit optional variadic arguments.
	required := c.Parameters
	if c.Variadic {
		required--
	}
	switch required {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, errors.Errorf("package %s NewFactory has unsupported arity %d", pkgPath, required)
	}
}
