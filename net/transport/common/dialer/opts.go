package dialer

// Validate validates the dialer options.
func (o *DialerOpts) Validate() error {
	if o.GetAddress() == "" {
		return ErrEmptyAddress
	}
	return nil
}
