package authorization

import (
	"strings"

	"github.com/amirphl/Yamata-no-Orochi/models"
)

// EffectivePermissions returns the resolved permission set for the given admin.
// Resolution order: role grants -> explicit denies -> explicit allows.
func EffectivePermissions(admin *models.Admin) map[PermissionKey]bool {
	perms := make(map[PermissionKey]bool)

	// Role grants
	for _, r := range admin.Roles {
		role := Role(strings.ToLower(r))
		if granted, ok := RolePermissions[role]; ok {
			for _, p := range granted {
				perms[p] = true
			}
		}
	}

	// Explicit denies
	for _, p := range admin.DeniedPermissions {
		perms[PermissionKey(strings.ToLower(p))] = false
	}

	// Explicit allows
	for _, p := range admin.AllowedPermissions {
		perms[PermissionKey(strings.ToLower(p))] = true
	}

	return perms
}

// HasPermission resolves permissions and returns true when the admin
// has the requested permission according to the resolution rules.
func HasPermission(admin *models.Admin, permission PermissionKey) bool {
	perms := EffectivePermissions(admin)
	allowed, ok := perms[PermissionKey(strings.ToLower(string(permission)))]
	if !ok {
		return false
	}
	return allowed
}
