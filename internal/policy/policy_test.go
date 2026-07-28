package policy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"keeper/pkg/auth"
	"keeper/pkg/httpclient"
)

func strPtr(s string) *string { return &s }

func TestCompileGroupsByRoleAndMarksSudo(t *testing.T) {
	rows := []Row{
		{Role: "ant_manager", Resource: strPtr("order"), Action: strPtr("update"), Scope: strPtr("own")},
		{Role: "ant_manager", Resource: strPtr("order"), Action: strPtr("read"), Scope: strPtr("any")},
		{Role: "sysadmin", IsSudo: true},
	}

	m := Compile(rows)

	manager, ok := m["ant_manager"]
	if !ok || manager.IsSudo || len(manager.Permissions) != 2 {
		t.Fatalf("ant_manager: got %+v", manager)
	}
	if manager.Permissions[0].Resource != "order" || manager.Permissions[0].Scope != "own" {
		t.Fatalf("unexpected first permission: %+v", manager.Permissions[0])
	}

	sudo, ok := m["sysadmin"]
	if !ok || !sudo.IsSudo || len(sudo.Permissions) != 0 {
		t.Fatalf("sysadmin: got %+v", sudo)
	}
}

func newTestServer(t *testing.T, rows []Row) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":` + string(body) + `}`))
	}))
}

func TestStoreWarmAndServeFromCache(t *testing.T) {
	srv := newTestServer(t, []Row{{Role: "sysadmin", IsSudo: true}})
	defer srv.Close()

	jwt := auth.NewJWTManager("secret", time.Hour)
	client := httpclient.New(httpclient.Config{Timeout: time.Second, Name: "test-policy"})
	fetcher := NewFetcher(client, srv.URL, 1, jwt)
	store := NewStore(fetcher, time.Hour)

	if err := store.Warm(context.Background()); err != nil {
		t.Fatalf("warm: %v", err)
	}

	m := store.Policies(context.Background())
	if !m["sysadmin"].IsSudo {
		t.Fatalf("want sysadmin sudo policy cached, got %+v", m)
	}

	srv.Close() // cache should still serve without hitting falcon again
	m = store.Policies(context.Background())
	if !m["sysadmin"].IsSudo {
		t.Fatalf("want cached policy served after falcon went away, got %+v", m)
	}
}

func TestStoreFailsClosedWhenFalconUnreachable(t *testing.T) {
	jwt := auth.NewJWTManager("secret", time.Hour)
	client := httpclient.New(httpclient.Config{Timeout: 200 * time.Millisecond, Name: "test-policy-down"})
	fetcher := NewFetcher(client, "http://127.0.0.1:1", 1, jwt)
	store := NewStore(fetcher, time.Hour)

	if err := store.Warm(context.Background()); err == nil {
		t.Fatal("want warm error when falcon unreachable")
	}

	m := store.Policies(context.Background())
	if len(m) != 0 {
		t.Fatalf("want empty (fail-closed) map, got %+v", m)
	}
}
