package rbac

import "testing"

func TestSubscriptionRolesRequireResourceGrants(t *testing.T) {
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
