package rbac

// Resource type constants.
const (
	ResourceTypeSharedObject   = "SharedObject"
	ResourceTypeBlockStore     = "BlockStore"
	ResourceTypeOrganization   = "Organization"
	ResourceTypeBillingAccount = "BillingAccount"
	ResourceTypeSession        = "Session"
	ResourceTypePlatform       = "Platform"
)

// SharedObject verbs.
const (
	VerbRead         = "read"
	VerbWriteOps     = "write_ops"
	VerbValidate     = "validate"
	VerbManageConfig = "manage_config"
	VerbTransfer     = "transfer"
)

// BlockStore verbs.
const (
	VerbPush   = "push"
	VerbPull   = "pull"
	VerbManage = "manage"
)

// Organization verbs.
const (
	VerbView          = "view"
	VerbManageMembers = "manage_members"
	VerbManageBilling = "manage_billing"
	VerbManageSpaces  = "manage_spaces"
)

// BillingAccount verbs.
const (
	VerbManageSubscription = "manage_subscription"
	VerbManagePayment      = "manage_payment"
)

// Session verbs.
const (
	VerbCreate = "create"
	VerbRevoke = "revoke"
)

// Platform verbs.
const VerbAdmin = "admin"

// VerbWildcard grants all verbs for a resource type.
const VerbWildcard = "*"
