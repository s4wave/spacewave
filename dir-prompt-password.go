package identity

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/promise"
)

// PromptPasswordCb is the callback to call with the result.
type PromptPasswordCb func(dir PromptPassword, val string)

// PromptPassword asks the user to enter a password to derive a key.
type PromptPassword interface {
	// Directive indicates this is a directive.
	directive.Directive

	// PromptPasswordDomainID is the identity domain id.
	PromptPasswordDomainID() string
	// PromptPasswordReason is the description to show users.
	PromptPasswordReason() string
	// PromptPasswordReasonDetail is additional description to show users.
	PromptPasswordReasonDetail() string
	// PromptPasswordCb is the callback to call with the result.
	PromptPasswordCb(val string)
	// PromptPasswordPrevError is the error for the previous attempt.
	// Usually nil.
	PromptPasswordPrevError() error
}

// PromptPasswordValue is a result of the PromptPassword directive.
// Note: this is not used, the callback is called instead.
type PromptPasswordValue struct{}

// ExPromptPassword executes the derive keypair directive.
//
// Returns the first value passed to the callback.
func ExPromptPassword(
	ctx context.Context,
	b bus.Bus,
	domainID, reason, reasonDetail string,
	prevErr error,
) (string, error) {
	result := promise.NewPromise[*string]()
	_, valRef, err := bus.ExecOneOff(
		ctx,
		b,
		NewPromptPassword(
			domainID, reason, reasonDetail,
			func(dir PromptPassword, val string) {
				result.SetResult(&val, nil)
			},
			prevErr,
		),
		true,
		nil,
	)
	if valRef != nil {
		valRef.Release()
	}
	if err != nil {
		return "", err
	}
	val, err := result.Await(ctx)
	if err != nil {
		return "", err
	}
	return *val, nil
}

// promptPassword implements PromptPassword
type promptPassword struct {
	domainID, reason string
	reasonDetail     string
	cb               PromptPasswordCb
	prevErr          error
}

// NewPromptPassword constructs a new PromptPassword directive.
func NewPromptPassword(domainID, reason, reasonDetail string, cb PromptPasswordCb, prevErr error) PromptPassword {
	return &promptPassword{
		domainID:     domainID,
		reason:       reason,
		reasonDetail: reasonDetail,
		cb:           cb,
		prevErr:      prevErr,
	}
}

// PromptPasswordDomainID is the identity domain id.
func (s *promptPassword) PromptPasswordDomainID() string {
	return s.domainID
}

// PromptPasswordReason is the description to show users.
func (s *promptPassword) PromptPasswordReason() string {
	return s.reason
}

// PromptPasswordReasonDetail is additional description to show users.
func (s *promptPassword) PromptPasswordReasonDetail() string {
	return s.reasonDetail
}

// PromptPasswordCb is the callback to call with the result.
func (s *promptPassword) PromptPasswordCb(val string) {
	if s.cb != nil {
		s.cb(s, val)
	}
}

// PromptPasswordPrevError is the error for the previous attempt.
// Usually nil.
func (s *promptPassword) PromptPasswordPrevError() error {
	return s.prevErr
}

// Validate checks the directive.
func (s *promptPassword) Validate() error {
	return nil
}

// GetValueLookupLoggerOptions returns options relating to value handling.
func (s *promptPassword) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (s *promptPassword) IsEquivalent(other directive.Directive) bool {
	return false
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (s *promptPassword) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (s *promptPassword) GetName() string {
	return "PromptPassword"
}

// GetDebugString returns the directive arguments stringified.
func (s *promptPassword) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if domainID := s.PromptPasswordDomainID(); domainID != "" {
		vals["domain-id"] = []string{domainID}
	}
	return vals
}

// _ is a type assertion
var _ PromptPassword = ((*promptPassword)(nil))
