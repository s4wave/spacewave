package identity_domain

import (
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupIdentityDomain looks up all available DomainInfo.
type LookupIdentityDomain interface {
	// Directive indicates this is a directive.
	directive.Directive

	// LookupIdentityDomainId filters by domain id.
	// Can be empty.
	LookupIdentityDomainId() string
}

// LookupIdentityDomainValue is the result of the LookupIdentityDomain directive.
type LookupIdentityDomainValue = *DomainInfo

// lookupIdentityDomain implements LookupIdentityDomain
type lookupIdentityDomain struct {
	// domainID filters the domain id
	domainID string
}

// NewLookupIdentityDomain constructs a new LookupIdentityDomain directive.
func NewLookupIdentityDomain(domainID string) LookupIdentityDomain {
	return &lookupIdentityDomain{
		domainID: domainID,
	}
}

// LookupIdentityDomainId filters by domain id.
// Can be empty.
func (s *lookupIdentityDomain) LookupIdentityDomainId() string {
	return s.domainID
}

// Validate checks the directive.
func (s *lookupIdentityDomain) Validate() error { return nil }

// GetValueLookupLoggerOptions returns options relating to value handling.
func (s *lookupIdentityDomain) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (s *lookupIdentityDomain) IsEquivalent(other directive.Directive) bool {
	ot, ok := other.(LookupIdentityDomain)
	if !ok {
		return false
	}

	return ot.LookupIdentityDomainId() == s.LookupIdentityDomainId()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (s *lookupIdentityDomain) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (s *lookupIdentityDomain) GetName() string {
	return "LookupIdentityDomain"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (s *lookupIdentityDomain) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if id := s.LookupIdentityDomainId(); id != "" {
		vals["domain-id"] = []string{id}
	}
	return vals
}

// _ is a type assertion
var _ LookupIdentityDomain = ((*lookupIdentityDomain)(nil))
