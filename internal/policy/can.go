package policy

import (
	"context"

	"keeper/pkg/auth"
)

// assignment pairs one of a user's falcon-resolved roles with its compiled
// policy and per-assignment tenant scope, ready for a Can() check.
type assignment struct {
	RolePolicy
	AppID *int
}

// userRoleAssignments resolves claims.Roles against the locally-cached
// policy map. Falcon's export (behind Store) is already scoped to this
// service's FALCON_SERVICE_ID, so a role name absent from policies belongs
// to a different service and is silently skipped here — no separate
// per-assignment service filter is needed on the consumer side.
func userRoleAssignments(claims *auth.UserClaims, policies map[string]RolePolicy) []assignment {
	assignments := make([]assignment, 0, len(claims.Roles))
	for _, r := range claims.Roles {
		rp, ok := policies[r.Name]
		if !ok {
			continue
		}
		assignments = append(assignments, assignment{RolePolicy: rp, AppID: r.AppID})
	}
	return assignments
}

// hasPermission reports whether any permission grants resource+action.
// Tier 1 (coarse CRUD) ignores field/scope — a field-restricted permission
// (e.g. admin's app.update on base fields only) still grants the coarse
// action here; the specific field/ownership check is layered on by Tier 2/3
// (steps 7.2/7.3), reusing this same Can() call.
func hasPermission(perms []Permission, resource, action string) bool {
	for _, p := range perms {
		if p.Resource == resource && p.Action == action {
			return true
		}
	}
	return false
}

// Can reports whether the user identified by claims is authorized for
// action on resource, scoped to appID (the target record's tenant — usually
// but not always claims.AppID, e.g. a sysadmin acting on another tenant).
// field is accepted but unused until Tier 2 (step 7.2) lands, so call sites
// don't need to change signature when field-level checks are added.
//
// Tenant isolation and the sudo bypass are orthogonal gates and both must
// pass: a sudo assignment scoped to one app (AppID set) only bypasses the
// permission check for that app, never across tenants.
func Can(ctx context.Context, store *Store, claims *auth.UserClaims, appID int, resource, action, field string) bool {
	_ = field // reserved for Tier 2 field-level enforcement

	assignments := userRoleAssignments(claims, store.Policies(ctx))

	for _, a := range assignments {
		if !a.IsSudo {
			continue
		}
		if a.AppID == nil || *a.AppID == appID {
			return true
		}
	}
	for _, a := range assignments {
		if hasPermission(a.Permissions, resource, action) {
			return true
		}
	}
	return false
}
