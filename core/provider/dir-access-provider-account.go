package provider

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// AccessProviderAccount is a directive to access an account on a provider.
type AccessProviderAccount interface {
	// Directive indicates AccessProviderAccount is a directive.
	directive.Directive

	// AccessProviderID returns the provider id to lookup.
	AccessProviderID() string
	// AccessProviderAccountID returns the account id to lookup.
	AccessProviderAccountID() string
}

// AccessProviderAccountValue is the result type for AccessProviderAccount.
type AccessProviderAccountValue = ProviderAccount

// ExAccessProviderAccount executes a lookup for a single provider on the bus.
//
// id should be set to filter to a specific provider id
// If waitOne is set, waits for at least one value before returning.
// Returns when the directive becomes idle.
func ExAccessProviderAccount(
	ctx context.Context,
	b bus.Bus,
	providerID,
	accountID string,
	returnIfIdle bool,
	valDisposeCb func(),
) (ProviderAccount, directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[AccessProviderAccountValue](
		ctx,
		b,
		NewAccessProviderAccount(providerID, accountID),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCb,
	)
	if err != nil {
		return nil, nil, err
	}
	if av == nil {
		avRef.Release()
		return nil, nil, nil
	}
	return av.GetValue(), avRef, nil
}

// accessProviderAccount implements AccessProviderAccount
type accessProviderAccount struct {
	providerID string
	accountID  string
}

// NewAccessProviderAccount constructs a new AccessProviderAccount directive.
func NewAccessProviderAccount(providerID, accountID string) AccessProviderAccount {
	return &accessProviderAccount{
		providerID: providerID,
		accountID:  accountID,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *accessProviderAccount) Validate() error {
	return nil
}

// GetValueAccessProviderAccountOptions returns options relating to value handling.
func (d *accessProviderAccount) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// AccessProviderID returns the provider id to lookup.
func (d *accessProviderAccount) AccessProviderID() string {
	return d.providerID
}

// AccessProviderAccountID returns the account id to lookup.
func (d *accessProviderAccount) AccessProviderAccountID() string {
	return d.accountID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *accessProviderAccount) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(AccessProviderAccount)
	if !ok {
		return false
	}

	if d.AccessProviderID() != od.AccessProviderID() {
		return false
	}

	if d.AccessProviderAccountID() != od.AccessProviderAccountID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *accessProviderAccount) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *accessProviderAccount) GetName() string {
	return "AccessProviderAccount"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *accessProviderAccount) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.providerID != "" {
		vals["provider-id"] = []string{d.providerID}
	}
	if d.accountID != "" {
		vals["account-id"] = []string{d.accountID}
	}
	return vals
}

// _ is a type assertion
var _ AccessProviderAccount = ((*accessProviderAccount)(nil))
