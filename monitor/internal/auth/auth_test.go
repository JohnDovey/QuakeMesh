// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.4 - Phase 3 auth tests: default admin seed, login, forced
//           password change, and rate limiting.

package auth

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s := New(db)
	if err := s.EnsureDefaultAdmin(); err != nil {
		t.Fatalf("EnsureDefaultAdmin: %v", err)
	}
	return s
}

func TestEnsureDefaultAdmin_andLogin(t *testing.T) {
	s := testStore(t)
	token, mustChange, err := s.Login(DefaultUsername, DefaultPassword, "client1")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !mustChange {
		t.Fatal("expected must_change_password on default login")
	}
	if token == "" {
		t.Fatal("expected session token")
	}
	username, stillMust, ok := s.ValidateSession(token)
	if !ok || username != DefaultUsername || !stillMust {
		t.Fatalf("session = (%q, %v, %v)", username, stillMust, ok)
	}
}

func TestChangePassword_rejectsDefaultReuse(t *testing.T) {
	s := testStore(t)
	if err := s.ChangePassword(DefaultUsername, DefaultPassword, DefaultPassword); err == nil {
		t.Fatal("expected error reusing default password")
	}
}

func TestChangePassword_andSubsequentLogin(t *testing.T) {
	s := testStore(t)
	const newPass = "secure-pass-1"
	if err := s.ChangePassword(DefaultUsername, DefaultPassword, newPass); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, _, err := s.Login(DefaultUsername, DefaultPassword, "client1"); err != ErrInvalidCredentials {
		t.Fatalf("old password login = %v, want ErrInvalidCredentials", err)
	}
	token, mustChange, err := s.Login(DefaultUsername, newPass, "client1")
	if err != nil || mustChange {
		t.Fatalf("new password login = token %q mustChange %v err %v", token, mustChange, err)
	}
}

func TestLogin_lockoutAfterFiveFailures(t *testing.T) {
	s := testStore(t)
	client := "locked-client"
	for i := 0; i < maxLoginFailures; i++ {
		if _, _, err := s.Login(DefaultUsername, "wrong", client); err != ErrInvalidCredentials {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	if _, _, err := s.Login(DefaultUsername, DefaultPassword, client); err != ErrLockedOut {
		t.Fatalf("expected lockout, got %v", err)
	}
	// Lockout expires after lockoutDuration; we cannot wait 60s in unit tests,
	// so just verify the locked state is active immediately after threshold.
	if time.Now().Add(lockoutDuration).Before(time.Now()) {
		t.Fatal("lockout duration misconfigured")
	}
}
