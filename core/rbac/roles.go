package rbac

// Built-in role IDs.
const (
	RoleAdmin              = "admin"
	RoleSubscriber         = "subscriber"
	RoleSubscriberReadonly = "subscriber_readonly"
	RoleOwner              = "owner"
	RoleEditor             = "editor"
	RoleViewer             = "viewer"
)

// SOParticipantRoleToRbacRole maps SOParticipantRole enum values to
// RBAC role IDs for syncRoleBindings.
var SOParticipantRoleToRbacRole = map[int32]string{
	4: RoleOwner,  // OWNER
	3: RoleEditor, // VALIDATOR
	2: RoleEditor, // WRITER
	1: RoleViewer, // READER
}

// SOParticipantRoleRequiredVerbs maps SOParticipantRole enum values
// to the RBAC verbs required for the verb containment check.
var SOParticipantRoleRequiredVerbs = map[int32][]string{
	4: {VerbRead, VerbWriteOps, VerbValidate, VerbManageConfig},
	3: {VerbRead, VerbWriteOps, VerbValidate},
	2: {VerbRead, VerbWriteOps},
	1: {VerbRead},
}

// BuiltinRoles returns all built-in role definitions.
func BuiltinRoles() []*RbacRole {
	return []*RbacRole{
		{
			Id:          RoleAdmin,
			DisplayName: "Admin",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypePlatform, Verbs: []string{VerbWildcard}},
				{ResourceType: ResourceTypeOrganization, Verbs: []string{VerbWildcard}},
				{ResourceType: ResourceTypeSharedObject, Verbs: []string{VerbWildcard}},
				{ResourceType: ResourceTypeBlockStore, Verbs: []string{VerbWildcard}},
				{ResourceType: ResourceTypeBillingAccount, Verbs: []string{VerbWildcard}},
				{ResourceType: ResourceTypeSession, Verbs: []string{VerbWildcard}},
			},
		},
		{
			Id:          RoleSubscriber,
			DisplayName: "Subscriber",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeSharedObject, Verbs: []string{VerbRead, VerbWriteOps}},
				{ResourceType: ResourceTypeBlockStore, Verbs: []string{VerbRead, VerbPush, VerbPull}},
				{ResourceType: ResourceTypeSession, Verbs: []string{VerbCreate}},
			},
		},
		{
			Id:          RoleSubscriberReadonly,
			DisplayName: "Subscriber (Read-only)",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeSharedObject, Verbs: []string{VerbRead}},
				{ResourceType: ResourceTypeBlockStore, Verbs: []string{VerbRead, VerbPull}},
			},
		},
		{
			Id:          RoleOwner,
			DisplayName: "Owner",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeSharedObject, Verbs: []string{VerbWildcard}},
				{ResourceType: ResourceTypeBlockStore, Verbs: []string{VerbWildcard}},
			},
		},
		{
			Id:          RoleEditor,
			DisplayName: "Editor",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeSharedObject, Verbs: []string{VerbRead, VerbWriteOps}},
				{ResourceType: ResourceTypeBlockStore, Verbs: []string{VerbRead, VerbPush, VerbPull}},
			},
		},
		{
			Id:          RoleViewer,
			DisplayName: "Viewer",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeSharedObject, Verbs: []string{VerbRead}},
				{ResourceType: ResourceTypeBlockStore, Verbs: []string{VerbRead, VerbPull}},
			},
		},
	}
}
