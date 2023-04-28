package cliutil

import (
	"fmt"

	"github.com/restic/chunker"
	ucli "github.com/urfave/cli/v2"
)

// RunGenerateRabinPoly generates a rabin polynomial.
func (a *UtilArgs) RunGenerateRabinPoly(cctx *ucli.Context) error {
	poly, err := chunker.RandomPolynomial()
	if err != nil {
		return err
	}
	fmt.Printf("%#v\n", poly)
	return nil
}
