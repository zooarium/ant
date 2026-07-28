// Package policy compiles falcon's role-permission export into a local,
// TTL-cached map for Tier 1 (coarse CRUD) authorization checks. It only
// builds the map here — checking a request against it is step 7.
package policy

// Row is one role-permission pair from falcon's policy export
// (GET /services/{id}/permissions/map). Resource/Action/Field/Scope are nil
// for a role with no granted permissions (e.g. a sudo role, which bypasses
// the map entirely) — the role still appears once so consumers know it exists.
type Row struct {
	Role          string  `json:"role"`
	Resource      *string `json:"resource"`
	Action        *string `json:"action"`
	Field         *string `json:"field,omitempty"`
	Scope         *string `json:"scope"`
	IsSudo        bool    `json:"is_sudo"`
	SudoServiceID *int    `json:"sudo_service_id"`
}

// Permission is one resource/action grant a role carries, optionally
// restricted to a single field or to "own" records.
type Permission struct {
	Resource string
	Action   string
	Field    string
	Scope    string
}

// RolePolicy is one role's compiled entry. IsSudo roles carry no
// permissions — the check in step 7 bypasses the map entirely for them.
type RolePolicy struct {
	IsSudo      bool
	Permissions []Permission
}

// Compile groups export rows by role. Union across a user's multiple roles
// happens at check time (step 7), not here.
func Compile(rows []Row) map[string]RolePolicy {
	m := make(map[string]RolePolicy, len(rows))
	for _, row := range rows {
		rp := m[row.Role]
		rp.IsSudo = rp.IsSudo || row.IsSudo
		if row.Resource != nil && row.Action != nil {
			p := Permission{Resource: *row.Resource, Action: *row.Action}
			if row.Field != nil {
				p.Field = *row.Field
			}
			if row.Scope != nil {
				p.Scope = *row.Scope
			}
			rp.Permissions = append(rp.Permissions, p)
		}
		m[row.Role] = rp
	}
	return m
}
