package world

// AuthenticatedOperation uses the transaction's authenticated signer as sender.
// Serialized delegated sender fields are ignored for these operations.
type AuthenticatedOperation interface {
	Operation
	AuthenticatedOperation()
}
