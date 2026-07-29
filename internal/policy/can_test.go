package policy

import (
	"context"
	"testing"
	"time"

	"keeper/pkg/auth"
	"keeper/pkg/httpclient"
)

// newTestStore warms a Store from the given rows via a throwaway falcon
// stand-in, mirroring the Worked Example in falcon/docs/rbac-plan.md.
func newTestStore(t *testing.T, rows []Row) *Store {
	t.Helper()
	srv := newTestServer(t, rows)
	t.Cleanup(srv.Close)

	jwt := auth.NewJWTManager("secret", time.Hour)
	client := httpclient.New(httpclient.Config{Timeout: time.Second, Name: "test-can"})
	store := NewStore(NewFetcher(client, srv.URL, 1, jwt), time.Hour)
	if err := store.Warm(context.Background()); err != nil {
		t.Fatalf("warm: %v", err)
	}
	return store
}

func appIDPtr(id int) *int { return &id }

// worked-example policy export: falcon/docs/rbac-plan.md "Worked Example".
var workedExampleRows = []Row{
	{Role: "sysadmin", IsSudo: true},
	{Role: "admin", Resource: strPtr("app"), Action: strPtr("update"), Scope: strPtr("any")},
	{Role: "ant_manager", Resource: strPtr("order"), Action: strPtr("read"), Scope: strPtr("own")},
	{Role: "ant_manager", Resource: strPtr("order"), Action: strPtr("update"), Scope: strPtr("own")},
	{Role: "ant_admin", IsSudo: true},
	{Role: "billing_manager", Resource: strPtr("pricing"), Action: strPtr("update"), Scope: strPtr("any")},
}

func TestCan_WorkedExample(t *testing.T) {
	store := newTestStore(t, workedExampleRows)
	ctx := context.Background()

	tests := []struct {
		name     string
		roles    []auth.RoleAssignment
		appID    int
		resource string
		action   string
		want     bool
	}{
		{
			name:     "global sudo bypasses everything",
			roles:    []auth.RoleAssignment{{Name: "sysadmin"}}, // user 501, app_id NULL
			appID:    999,
			resource: "anything",
			action:   "anything",
			want:     true,
		},
		{
			name:     "field-restricted admin still passes coarse check",
			roles:    []auth.RoleAssignment{{Name: "admin"}}, // user 502
			appID:    1,
			resource: "app",
			action:   "update",
			want:     true, // field/ownership deferred to Tier 2/3
		},
		{
			name:     "admin has no grant outside its permission",
			roles:    []auth.RoleAssignment{{Name: "admin"}},
			appID:    1,
			resource: "app",
			action:   "delete",
			want:     false,
		},
		{
			name:     "ownership-scoped manager passes coarse action check",
			roles:    []auth.RoleAssignment{{Name: "ant_manager"}}, // user 503
			appID:    1,
			resource: "order",
			action:   "read",
			want:     true, // ownership filter itself is Tier 3, not Can()
		},
		{
			name:     "manager has no grant on unrelated resource",
			roles:    []auth.RoleAssignment{{Name: "ant_manager"}},
			appID:    1,
			resource: "pricing",
			action:   "update",
			want:     false,
		},
		{
			name:     "service-scoped sudo (ant_admin) bypasses within its service",
			roles:    []auth.RoleAssignment{{Name: "ant_admin"}}, // user 504
			appID:    1,
			resource: "order",
			action:   "delete",
			want:     true,
		},
		{
			name:     "single-permission role matches its one grant",
			roles:    []auth.RoleAssignment{{Name: "billing_manager"}}, // user 505
			appID:    1,
			resource: "pricing",
			action:   "update",
			want:     true,
		},
		{
			name:     "single-permission role rejects everything else",
			roles:    []auth.RoleAssignment{{Name: "billing_manager"}},
			appID:    1,
			resource: "app",
			action:   "update",
			want:     false,
		},
		{
			name:     "app-owner sudo passes for its own tenant",
			roles:    []auth.RoleAssignment{{Name: "sysadmin", AppID: appIDPtr(42)}}, // user 506
			appID:    42,
			resource: "anything",
			action:   "anything",
			want:     true,
		},
		{
			name:     "app-owner sudo denied outside its tenant",
			roles:    []auth.RoleAssignment{{Name: "sysadmin", AppID: appIDPtr(42)}},
			appID:    99,
			resource: "anything",
			action:   "anything",
			want:     false,
		},
		{
			name: "union across multiple roles: either match passes",
			roles: []auth.RoleAssignment{
				{Name: "billing_manager"},
				{Name: "ant_manager"},
			},
			appID:    1,
			resource: "order",
			action:   "update",
			want:     true,
		},
		{
			name:     "role absent from local policy map (different service) grants nothing",
			roles:    []auth.RoleAssignment{{Name: "unknown_role"}},
			appID:    1,
			resource: "app",
			action:   "update",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &auth.UserClaims{Roles: tt.roles}
			got := Can(ctx, store, claims, tt.appID, tt.resource, tt.action, "")
			if got != tt.want {
				t.Fatalf("Can() = %v, want %v", got, tt.want)
			}
		})
	}
}
