// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.4 - Phase 3: session auth, default Admin account seeding,
//           forced password change, and login rate limiting.

// Package auth implements QuakeMeshMonitor's admin login, sessions, and
// password management. See "Authentication" under QuakeMeshMonitor in
// /plan.md.
package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/JohnDovey/QuakeMesh/core/storage"
)

const (
	DefaultUsername        = "Admin"
	DefaultPassword        = "test1234"
	bcryptCost             = bcrypt.DefaultCost
	sessionCookieName      = "quakemesh_session"
	maxLoginFailures       = 5
	lockoutDuration        = 60 * time.Second
	sessionLifetime        = 24 * time.Hour
)

// ErrInvalidCredentials is returned when username or password is wrong.
var ErrInvalidCredentials = errors.New("invalid username or password")

// ErrLockedOut is returned when too many failed login attempts occurred.
var ErrLockedOut = errors.New("too many failed login attempts; try again later")

// ErrMustChangePassword is returned when the session must change password first.
var ErrMustChangePassword = errors.New("password change required")

// Store manages admin credentials in SQLite and in-memory sessions.
type Store struct {
	db *storage.DB

	mu       sync.Mutex
	sessions map[string]session
	attempts map[string]*loginAttempts
}

type session struct {
	username            string
	mustChangePassword  bool
	expiresAt           time.Time
}

type loginAttempts struct {
	failures  int
	lockedUntil time.Time
}

// New creates an auth Store backed by the Hub's SQLite database.
func New(db *storage.DB) *Store {
	return &Store{
		db:       db,
		sessions: make(map[string]session),
		attempts: make(map[string]*loginAttempts),
	}
}

// SessionCookieName is the HTTP cookie name used for authenticated sessions.
func SessionCookieName() string { return sessionCookieName }

// EnsureDefaultAdmin creates the default Admin/test1234 account when the
// admin_users table is empty.
func (s *Store) EnsureDefaultAdmin() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM admin_users`).Scan(&count); err != nil {
		return fmt.Errorf("auth: count admin_users: %w", err)
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(DefaultPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("auth: hash default password: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO admin_users (username, password_hash, salt, must_change_password) VALUES (?, ?, ?, 1)`,
		DefaultUsername, hash, []byte{},
	)
	if err != nil {
		return fmt.Errorf("auth: seed default admin: %w", err)
	}
	return nil
}

// Login validates credentials and returns a new session token.
func (s *Store) Login(username, password, clientKey string) (token string, mustChange bool, err error) {
	if err := s.checkLockout(clientKey); err != nil {
		return "", false, err
	}

	var hash []byte
	var mustChangeInt int
	err = s.db.QueryRow(
		`SELECT password_hash, must_change_password FROM admin_users WHERE username = ?`,
		username,
	).Scan(&hash, &mustChangeInt)
	if errors.Is(err, sql.ErrNoRows) {
		s.recordFailure(clientKey)
		return "", false, ErrInvalidCredentials
	}
	if err != nil {
		return "", false, fmt.Errorf("auth: lookup user: %w", err)
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		s.recordFailure(clientKey)
		return "", false, ErrInvalidCredentials
	}

	s.clearFailures(clientKey)

	token, err = newToken()
	if err != nil {
		return "", false, err
	}

	s.mu.Lock()
	s.sessions[token] = session{
		username:           username,
		mustChangePassword: mustChangeInt != 0,
		expiresAt:          time.Now().Add(sessionLifetime),
	}
	s.mu.Unlock()

	return token, mustChangeInt != 0, nil
}

// ChangePassword updates the user's password and clears must_change_password.
func (s *Store) ChangePassword(username, currentPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if currentPassword == newPassword {
		return errors.New("new password must differ from the current password")
	}
	if username == DefaultUsername && newPassword == DefaultPassword {
		return errors.New("default password cannot be reused")
	}

	var hash []byte
	if err := s.db.QueryRow(
		`SELECT password_hash FROM admin_users WHERE username = ?`, username,
	).Scan(&hash); err != nil {
		return fmt.Errorf("auth: lookup user: %w", err)
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(currentPassword)) != nil {
		return ErrInvalidCredentials
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("auth: hash new password: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE admin_users SET password_hash = ?, must_change_password = 0 WHERE username = ?`,
		newHash, username,
	)
	if err != nil {
		return fmt.Errorf("auth: update password: %w", err)
	}

	s.mu.Lock()
	for token, sess := range s.sessions {
		if sess.username == username {
			sess.mustChangePassword = false
			s.sessions[token] = sess
		}
	}
	s.mu.Unlock()
	return nil
}

// ValidateSession returns the username and whether a password change is
// still required. ok is false for missing or expired sessions.
func (s *Store) ValidateSession(token string) (username string, mustChange bool, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, found := s.sessions[token]
	if !found || time.Now().After(sess.expiresAt) {
		if found {
			delete(s.sessions, token)
		}
		return "", false, false
	}
	return sess.username, sess.mustChangePassword, true
}

// Logout removes a session token.
func (s *Store) Logout(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func (s *Store) checkLockout(clientKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	att := s.attempts[clientKey]
	if att != nil && time.Now().Before(att.lockedUntil) {
		return ErrLockedOut
	}
	return nil
}

func (s *Store) recordFailure(clientKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	att := s.attempts[clientKey]
	if att == nil {
		att = &loginAttempts{}
		s.attempts[clientKey] = att
	}
	att.failures++
	if att.failures >= maxLoginFailures {
		att.lockedUntil = time.Now().Add(lockoutDuration)
		att.failures = 0
	}
}

func (s *Store) clearFailures(clientKey string) {
	s.mu.Lock()
	delete(s.attempts, clientKey)
	s.mu.Unlock()
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: session token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
