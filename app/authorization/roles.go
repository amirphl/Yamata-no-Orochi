package authorization

// Role defines a coarse-grained permission bundle for admins.
type Role string

// PermissionKey identifies a fine-grained permission tied to a feature/action.
type PermissionKey string

const (
    RoleSuperAdmin Role = "superadmin"
    RoleFinance    Role = "finance"
    RoleSupport    Role = "support"
    RoleContent    Role = "content"
    RoleReadOnly   Role = "readonly"
)

const (
    PermissionCampaignRead          PermissionKey = "campaign:read"
    PermissionCampaignWrite         PermissionKey = "campaign:write"
    PermissionCampaignApprove       PermissionKey = "campaign:approve"
    PermissionPaymentReceiptReview  PermissionKey = "payment:receipt_review"
    PermissionPaymentChargeWallet   PermissionKey = "payment:charge_wallet"
    PermissionPaymentRead           PermissionKey = "payment:read"
    PermissionUserList              PermissionKey = "user:list"
    PermissionUserWrite             PermissionKey = "user:write"
    PermissionPlatformBasePriceRead PermissionKey = "platform-base-price:read"
    PermissionPlatformBasePriceEdit PermissionKey = "platform-base-price:create"
    PermissionShortLinkManage       PermissionKey = "shortlink:manage"
    PermissionLineNumberRead        PermissionKey = "line-number:read"
    PermissionLineNumberWrite       PermissionKey = "line-number:write"
    PermissionLineNumberReport      PermissionKey = "line-number:report"
    PermissionTicketRead            PermissionKey = "ticket:read"
    PermissionTicketReply           PermissionKey = "ticket:reply"
    PermissionMediaRead             PermissionKey = "media:read"
	PermissionPlatformSettingsRead  PermissionKey = "platform-settings:read"
	PermissionPlatformSettingsWrite PermissionKey = "platform-settings:write"
	PermissionACLManage             PermissionKey = "acl:manage"
	PermissionACLApprove            PermissionKey = "acl:approve"
)

// PermissionCatalog documents available permissions with a short description.
var PermissionCatalog = map[PermissionKey]string{
    PermissionCampaignRead:          "View campaign lists and details",
    PermissionCampaignWrite:         "Modify campaign resources (non-approval actions)",
    PermissionCampaignApprove:       "Approve / reject / reschedule / cancel campaigns",
    PermissionPaymentReceiptReview:  "Review and act on deposit receipts",
    PermissionPaymentChargeWallet:   "Charge wallets on behalf of customers",
    PermissionPaymentRead:           "View payment and wallet information",
    PermissionUserList:              "List or view customers and related reports",
    PermissionUserWrite:             "Change customer status or attributes",
    PermissionPlatformBasePriceRead: "Read platform base/segment price factors",
    PermissionPlatformBasePriceEdit: "Create or update platform base/segment price factors",
    PermissionShortLinkManage:       "Upload/export short-links and click data",
    PermissionLineNumberRead:        "View line number inventory and reports",
    PermissionLineNumberWrite:       "Create or update line number inventory",
    PermissionLineNumberReport:      "Export/report on line numbers",
    PermissionTicketRead:            "List and read support tickets",
    PermissionTicketReply:           "Respond to support tickets",
    PermissionMediaRead:             "Download or preview stored media",
    PermissionPlatformSettingsRead:  "Read platform settings",
    PermissionPlatformSettingsWrite: "Update platform settings or metadata",
	PermissionACLManage:             "Create ACL change requests (maker)",
	PermissionACLApprove:            "Approve or reject ACL change requests (checker)",
}

// RolePermissions maps roles to the permissions they grant by default.
var RolePermissions = map[Role][]PermissionKey{
    RoleSuperAdmin: {
        PermissionCampaignRead,
        PermissionCampaignWrite,
        PermissionCampaignApprove,
        PermissionPaymentReceiptReview,
        PermissionPaymentChargeWallet,
        PermissionPaymentRead,
        PermissionUserList,
        PermissionUserWrite,
        PermissionPlatformBasePriceRead,
        PermissionPlatformBasePriceEdit,
        PermissionShortLinkManage,
        PermissionLineNumberRead,
        PermissionLineNumberWrite,
        PermissionLineNumberReport,
        PermissionTicketRead,
        PermissionTicketReply,
        PermissionMediaRead,
        PermissionPlatformSettingsRead,
		PermissionPlatformSettingsWrite,
		PermissionACLManage,
		PermissionACLApprove,
	},
    RoleFinance: {
        PermissionPaymentReceiptReview,
        PermissionPaymentChargeWallet,
        PermissionPaymentRead,
        PermissionUserList,
    },
    RoleSupport: {
        PermissionTicketRead,
        PermissionTicketReply,
        PermissionUserList,
        PermissionCampaignRead,
    },
    RoleContent: {
        PermissionShortLinkManage,
        PermissionMediaRead,
        PermissionCampaignRead,
        PermissionPlatformSettingsRead,
    },
    RoleReadOnly: {
        PermissionCampaignRead,
        PermissionPaymentRead,
        PermissionUserList,
        PermissionPlatformBasePriceRead,
        PermissionLineNumberRead,
        PermissionPlatformSettingsRead,
        PermissionTicketRead,
    },
}

// DefaultRoles is applied when an admin has no explicit assignment.
// Keep this intentionally empty to enforce least-privilege defaults.
var DefaultRoles = []Role{}
