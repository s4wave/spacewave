package git_block

import (
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// NewCheckoutOpts constructs a new CheckoutOpts from a git checkout opts.
func NewCheckoutOpts(o *git.CheckoutOptions) (*CheckoutOpts, error) {
	if o == nil {
		return &CheckoutOpts{}, nil
	}

	commitHash, err := NewHash(o.Hash)
	if err != nil {
		return nil, err
	}

	return &CheckoutOpts{
		Commit: commitHash,
		Branch: string(o.Branch),
		Create: o.Create,
		Force:  o.Force,
		Keep:   o.Keep,
	}, nil
}

// Validate validates the checkout opts.
func (o *CheckoutOpts) Validate() error {
	if err := o.GetCommit().Validate(); err != nil {
		return errors.Wrap(err, "commit")
	}
	if !o.GetCommit().IsEmpty() {
		if o.GetBranch() != "" && !o.GetCreate() {
			return errors.New("commit and branch cannot both be set unless create is set")
		}
		// enforce sha1 to fit plumbing.Hash
		if err := ValidateHash(o.GetCommit()); err != nil {
			return err
		}
	}
	return nil
}

// BuildCheckoutOpts constructs git checkout opts.
func (o *CheckoutOpts) BuildCheckoutOpts() (*git.CheckoutOptions, error) {
	var err error
	var checkoutHash plumbing.Hash
	commitEmpty := o.GetCommit().IsEmpty()
	if !commitEmpty {
		checkoutHash, err = FromHash(o.GetCommit())
		if err != nil {
			return nil, err
		}
	}
	return &git.CheckoutOptions{
		Branch: plumbing.ReferenceName(o.GetBranch()),
		Hash:   checkoutHash,
		Create: o.GetCreate(),
		Force:  o.GetForce(),
		Keep:   o.GetKeep(),
	}, nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *CheckoutOpts) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *CheckoutOpts) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*CheckoutOpts)(nil))
