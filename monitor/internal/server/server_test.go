// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.4 - Phase 3 HTTP tests: login flow, forced password change
//           gate, and overview API.

package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/JohnDovey/QuakeMesh/core/storage"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/auth"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/datastore"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/hubclient"
)

//go:embed static
var testStatic embed.FS

func testServer(t *testing.T) (*Server, *auth.Store) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	authStore := auth.New(db)
	if err := authStore.EnsureDefaultAdmin(); err != nil {
		t.Fatalf("EnsureDefaultAdmin: %v", err)
	}
	s := New(Config{
		BindAddr: "127.0.0.1:0",
		StaticFS: testStatic,
		Auth:     authStore,
		Data:     datastore.New(db),
		Hub:      hubclient.New("ws://127.0.0.1:9/ws"),
	})
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx := t.Context()
		_ = s.Close(ctx)
	})
	return s, authStore
}

func TestLogin_ForcedPasswordChangeBlocksOverview(t *testing.T) {
	s, _ := testServer(t)
	base := "http://" + s.Addr().String()
	body, _ := json.Marshal(map[string]string{
		"username": auth.DefaultUsername,
		"password": auth.DefaultPassword,
	})
	rec, err := http.Post(base+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer rec.Body.Close()
	if rec.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", rec.StatusCode)
	}
	cookies := rec.Cookies()

	req2, _ := http.NewRequest(http.MethodGet, base+"/api/overview", nil)
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	rec2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	defer rec2.Body.Close()
	if rec2.StatusCode != http.StatusForbidden {
		t.Fatalf("overview before password change = %d, want 403", rec2.StatusCode)
	}
}

func TestChangePassword_thenOverview(t *testing.T) {
	s, authStore := testServer(t)
	base := "http://" + s.Addr().String()
	if err := authStore.ChangePassword(auth.DefaultUsername, auth.DefaultPassword, "new-password-9"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	body, _ := json.Marshal(map[string]string{
		"username": auth.DefaultUsername,
		"password": "new-password-9",
	})
	rec, err := http.Post(base+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer rec.Body.Close()
	if rec.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", rec.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodGet, base+"/api/overview", nil)
	for _, c := range rec.Cookies() {
		req2.AddCookie(c)
	}
	rec2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	defer rec2.Body.Close()
	if rec2.StatusCode != http.StatusOK {
		t.Fatalf("overview = %d", rec2.StatusCode)
	}
}

func TestLogin_rejectsDefaultAfterPasswordChange(t *testing.T) {
	s, authStore := testServer(t)
	base := "http://" + s.Addr().String()
	_ = authStore.ChangePassword(auth.DefaultUsername, auth.DefaultPassword, "another-pass-9")
	body, _ := json.Marshal(map[string]string{
		"username": auth.DefaultUsername,
		"password": auth.DefaultPassword,
	})
	rec, err := http.Post(base+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer rec.Body.Close()
	if rec.StatusCode != http.StatusUnauthorized {
		t.Fatalf("default password login after change = %d", rec.StatusCode)
	}
}
