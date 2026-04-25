package bstore

import "errors"

var (
	// ErrInvalidBlockStoreParticipantRole is returned if the block store participant role is invalid.
	ErrInvalidBlockStoreParticipantRole = errors.New("invalid block store participant role")

	// ErrEmptyBlockStoreID is returned if the block store id was empty.
	ErrEmptyBlockStoreID = errors.New("block store id cannot be empty")

	// ErrBlockStoreExists is returned if the block store already exists.
	ErrBlockStoreExists = errors.New("block store with that id already exists")
)
