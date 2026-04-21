package pubmessage

// Validate checks the inner data.
func (i *PubMessageInner) Validate() error {
	if i.GetChannel() == "" {
		return ErrInvalidChannelID
	}
	// allow empty
	if ts := i.GetTimestamp(); ts.GetSeconds() != 0 {
		if err := ts.CheckValid(); err != nil {
			return err
		}
	}
	return nil
}
