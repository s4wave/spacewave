package resource

import "testing"

func TestRegisterResourceRPCAdoptingUnaryMethod(t *testing.T) {
	const (
		serviceID = "external.fixture.ResourceService"
		methodID  = "AccessChild"
	)

	if IsResourceRPCAdoptingUnaryMethod(serviceID, methodID) {
		t.Fatal("method is registered before RegisterResourceRPCAdoptingUnaryMethod")
	}

	RegisterResourceRPCAdoptingUnaryMethod(serviceID, methodID)
	RegisterResourceRPCAdoptingUnaryMethod(serviceID, methodID)

	if !IsResourceRPCAdoptingUnaryMethod(serviceID, methodID) {
		t.Fatal("method is not registered after RegisterResourceRPCAdoptingUnaryMethod")
	}
	if IsResourceRPCAdoptingUnaryMethod(serviceID, "OtherMethod") {
		t.Fatal("registration matched another method")
	}
}

func TestGeneratedResourceRPCAdoptingUnaryMethod(t *testing.T) {
	if !IsResourceRPCAdoptingUnaryMethod(
		"s4wave.root.RootResourceService",
		"AccessStateAtom",
	) {
		t.Fatal("generated adopting method is not registered")
	}
}
