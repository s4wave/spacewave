package world_cayley

import "errors"

var (
	// ErrObjNotIRI is returned if the format <object-id> is not used for the graph key.
	ErrObjNotIRI = errors.New("subject and object fields must be valid object IRIs")
)
