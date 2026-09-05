package rbac

// CheckAccessResult is the result of an access check.
type CheckAccessResult struct {
	// Allowed indicates whether access is granted.
	Allowed bool
	// MatchedBinding is the binding that granted access.
	MatchedBinding *RbacRoleBinding
}

// CheckAccess matches permission rules in an already-authorized set of bindings.
// The caller must filter by subject, scope, expiry, and revocation before calling.
// This helper is not an authorization boundary or a cloud grant evaluator.
func CheckAccess(
	roles []*RbacRole,
	bindings []*RbacRoleBinding,
	resourceType string,
	verb string,
) CheckAccessResult {
	// Match only the permissions carried by the supplied, prefiltered bindings.
	for _, b := range bindings {
		role := findRole(roles, b.GetRoleId())
		if role == nil {
			continue
		}
		for _, rule := range role.GetRules() {
			if rule.GetResourceType() != resourceType {
				continue
			}
			for _, v := range rule.GetVerbs() {
				if v == verb || v == VerbWildcard {
					return CheckAccessResult{
						Allowed:        true,
						MatchedBinding: b,
					}
				}
			}
		}
	}
	return CheckAccessResult{}
}

// findRole finds a role by ID in the given slice.
func findRole(roles []*RbacRole, id string) *RbacRole {
	for _, r := range roles {
		if r.GetId() == id {
			return r
		}
	}
	return nil
}
