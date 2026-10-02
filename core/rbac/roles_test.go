package rbac

import "testing"

// TestSubscriptionRolesRequireResourceGrants checks that subscription roles
// reach objects only through resource grants.
func TestSubscriptionRolesRequireResourceGrants(t *testing.T) {
	// Subscription roles grant no global object or block store read, but may create a Session.
	roles := BuiltinRoles()
	for _, id := range []string{RoleSubscriber, RoleSubscriberReadonly, "free"} {
		bindings := []*RbacRoleBinding{{RoleId: id}}
		for _, resource := range []string{ResourceTypeSharedObject, ResourceTypeBlockStore} {
			if CheckAccess(roles, bindings, resource, VerbRead).Allowed {
				t.Fatalf("%s grants global %s read", id, resource)
			}
		}
		if !CheckAccess(roles, bindings, ResourceTypeSession, VerbCreate).Allowed {
			t.Fatalf("%s cannot create a session", id)
		}
	}

	// A subscriber creates objects, and only a developer executes runtime capabilities.
	if !CheckAccess(roles, []*RbacRoleBinding{{RoleId: RoleSubscriber}}, ResourceTypeSharedObject, VerbCreate).Allowed {
		t.Fatal("subscriber cannot create objects")
	}
	if CheckAccess(roles, []*RbacRoleBinding{{RoleId: RoleAdmin}}, "RuntimeCapability", "execute").Allowed {
		t.Fatal("admin implicitly runs developer capabilities")
	}
	if !CheckAccess(roles, []*RbacRoleBinding{{RoleId: "developer"}}, "RuntimeCapability", "execute").Allowed {
		t.Fatal("developer cannot execute")
	}
}
