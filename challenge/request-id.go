package auth_challenge

// RequestID uniquely identifies a lookup request.
type RequestID [2]string

// NewRequestID builds a new request identifier.
func NewRequestID(domainID, entityID string) RequestID {
	return [2]string{
		domainID,
		entityID,
	}
}

// GetDomainID returns the domain ID.
func (r RequestID) GetDomainID() string {
	return r[0]
}

// GetEntityID returns the entity ID.
func (r RequestID) GetEntityID() string {
	return r[1]
}

// ToRequestID constructs a request identifier from the object.
func (i *EntityLookupIdentifier) ToRequestID() RequestID {
	return NewRequestID(i.GetDomainId(), i.GetEntityId())
}
