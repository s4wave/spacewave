package s4wave_org

// MarshalInitOrgSOOp wraps an InitOrganizationOp in OrgSOOp and marshals it.
func MarshalInitOrgSOOp(op *InitOrganizationOp) ([]byte, error) {
	return (&OrgSOOp{Body: &OrgSOOp_InitOrg{InitOrg: op}}).MarshalVT()
}

// MarshalUpdateOrgSOOp wraps an UpdateOrgOp in OrgSOOp and marshals it.
func MarshalUpdateOrgSOOp(op *UpdateOrgOp) ([]byte, error) {
	return (&OrgSOOp{Body: &OrgSOOp_UpdateOrg{UpdateOrg: op}}).MarshalVT()
}

// MarshalDeleteOrgSOOp wraps a DeleteOrganizationOp in OrgSOOp and marshals it.
func MarshalDeleteOrgSOOp(op *DeleteOrganizationOp) ([]byte, error) {
	return (&OrgSOOp{Body: &OrgSOOp_DeleteOrg{DeleteOrg: op}}).MarshalVT()
}
