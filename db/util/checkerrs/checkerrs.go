package checkerrs

// AnyErrors checks if any of the errors is not nil.
func AnyErrors(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
