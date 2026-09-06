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

// BuiltinRoles returns the built-in permission catalog. Cloud migrations own deployed roles.
// Subscription roles permit creation; resource access requires a scoped grant.
func BuiltinRoles() []*RbacRole {
	return []*RbacRole{
		{
			Id:          "developer",
			DisplayName: "Developer",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: "RuntimeCapability", Verbs: []string{"execute"}},
			},
		},
		{
			Id:          VerbAdmin,
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
			Id:          "release_notify",
			DisplayName: "Release notify",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypePlatform, Verbs: []string{"update_notify"}},
			},
		},
		{
			Id:          RoleSubscriber,
			DisplayName: "Subscriber",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeSharedObject, Verbs: []string{VerbCreate}},
				{ResourceType: ResourceTypeBlockStore, Verbs: []string{VerbCreate}},
				{ResourceType: ResourceTypeSession, Verbs: []string{VerbCreate}},
				{ResourceType: ResourceTypeOrganization, Verbs: []string{VerbView}},
			},
		},
		{
			Id:          RoleSubscriberReadonly,
			DisplayName: "Subscriber (Read-only)",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeSession, Verbs: []string{VerbCreate}},
				{ResourceType: ResourceTypeOrganization, Verbs: []string{VerbView}},
			},
		},
		{
			Id:          "free",
			DisplayName: "Free",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeSession, Verbs: []string{VerbCreate}},
				{ResourceType: ResourceTypeOrganization, Verbs: []string{VerbView}},
			},
		},
		{
			Id:          RoleOwner,
			DisplayName: "Owner",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeSharedObject, Verbs: []string{VerbRead, VerbWriteOps, VerbValidate, VerbManageConfig, VerbTransfer}},
				{ResourceType: ResourceTypeBlockStore, Verbs: []string{VerbRead, VerbPush, VerbPull, VerbManage, VerbTransfer}},
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
		{
			Id:          "org:owner",
			DisplayName: "Organization Owner",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeOrganization, Verbs: []string{VerbWildcard}},
				{ResourceType: ResourceTypeSharedObject, Verbs: []string{VerbWildcard}},
				{ResourceType: ResourceTypeBlockStore, Verbs: []string{VerbWildcard}},
			},
		},
		{
			Id:          "org:member",
			DisplayName: "Organization Member",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeOrganization, Verbs: []string{VerbView, VerbManageSpaces}},
				{ResourceType: ResourceTypeSharedObject, Verbs: []string{VerbRead}},
				{ResourceType: ResourceTypeBlockStore, Verbs: []string{VerbRead}},
			},
		},
		{
			Id:          "org:billing",
			DisplayName: "Organization Billing",
			Builtin:     true,
			Rules: []*RbacRule{
				{ResourceType: ResourceTypeOrganization, Verbs: []string{VerbManageBilling}},
				{ResourceType: ResourceTypeBillingAccount, Verbs: []string{VerbWildcard}},
			},
		},
		{
			Id:          "disputed_locked",
			DisplayName: "Disputed (Locked)",
			Builtin:     true,
			Rules:       []*RbacRule{},
		},
	}
}
