package hydra_git

import (
	"context"

	"github.com/pkg/errors"
)

// NewFuncEngine constructs an Engine from a transaction factory function.
func NewFuncEngine(
	buildTx func(ctx context.Context, write bool) (Tx, error),
) Engine {
	return &funcEngine{buildTx: buildTx}
}

type funcEngine struct {
	buildTx func(ctx context.Context, write bool) (Tx, error)
}

func (e *funcEngine) NewTransaction(ctx context.Context, write bool) (Tx, error) {
	if e == nil || e.buildTx == nil {
		return nil, errors.New("nil git transaction factory")
	}
	return e.buildTx(ctx, write)
}

// _ is a type assertion
var _ Engine = ((*funcEngine)(nil))
