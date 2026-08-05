package resource

import "sync"

type resourceRPCMethod struct {
	serviceID string
	methodID  string
}

var externalResourceRPCAdoptingUnaryMethods sync.Map

// RegisterResourceRPCAdoptingUnaryMethod registers an adopting unary method
// implemented outside the Spacewave protobuf tree.
func RegisterResourceRPCAdoptingUnaryMethod(serviceID, methodID string) {
	externalResourceRPCAdoptingUnaryMethods.Store(resourceRPCMethod{
		serviceID: serviceID,
		methodID:  methodID,
	}, struct{}{})
}

func isExternalResourceRPCAdoptingUnaryMethod(serviceID, methodID string) bool {
	_, ok := externalResourceRPCAdoptingUnaryMethods.Load(resourceRPCMethod{
		serviceID: serviceID,
		methodID:  methodID,
	})
	return ok
}
