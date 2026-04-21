package cliutil

import (
	"fmt"

	ucli "github.com/aperturerobotics/cli"
	"github.com/restic/chunker"
)

// RunGenerateRabinPoly generates a rabin polynomial.
func (a *UtilArgs) RunGenerateRabinPoly(cctx *ucli.Context) error {
	poly, err := chunker.RandomPolynomial()
	if err != nil {
		return err
	}
	fmt.Printf("%d\n", uint64(poly))
	return nil
}
