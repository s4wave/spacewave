# World Graph: Block Transactions

This package implements a wrapper around the block-graph backed World State
which forks the world state and applies changes to the temporary fork while
building a list of transactions which can be serialized and transmitted.

The transaction set can be applied again later.
