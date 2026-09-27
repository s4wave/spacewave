package world

import "context"

type operationPersonKey struct{}

// WithOperationPerson binds the accepted config's person to authenticated replay.
func WithOperationPerson(ctx context.Context, person string) context.Context {
	return context.WithValue(ctx, operationPersonKey{}, person)
}

// OperationPersonFromContext returns the person bound by the replay authority.
func OperationPersonFromContext(ctx context.Context) string {
	person, _ := ctx.Value(operationPersonKey{}).(string)
	return person
}
