package dto

// AdminACLChangeRequestCreate holds payload to propose a role/override change.
type AdminACLChangeRequestCreate struct {
	TargetAdminID uint     `json:"target_admin_id" validate:"required"`
	Roles         []string `json:"roles"`
	Allow         []string `json:"allow"`
	Deny          []string `json:"deny"`
	Reason        string   `json:"reason"`
}

// AdminACLChangeDecision represents approve/reject action.
type AdminACLChangeDecision struct {
	Reason string `json:"reason"`
}

// AdminACLChangeRequestResponse summarizes a change request.
type AdminACLChangeRequestResponse struct {
	UUID             string   `json:"uuid"`
	Status           string   `json:"status"`
	TargetAdminID    uint     `json:"target_admin_id"`
	RequestedByAdmin uint     `json:"requested_by_admin"`
	ApprovedByAdmin  *uint    `json:"approved_by_admin,omitempty"`
	RolesBefore      []string `json:"roles_before"`
	RolesAfter       []string `json:"roles_after"`
	AllowBefore      []string `json:"allow_before"`
	AllowAfter       []string `json:"allow_after"`
	DenyBefore       []string `json:"deny_before"`
	DenyAfter        []string `json:"deny_after"`
}
