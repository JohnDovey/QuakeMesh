// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.11 - Phase 9: hub config key-value store.

package configstore

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/JohnDovey/QuakeMesh/core/storage"
)

const KeyInternetFallbackEnabled = "internet_fallback_enabled"

// Store reads and writes the config table.
type Store struct {
	db *storage.DB
}

// New wraps a migrated storage.DB.
func New(db *storage.DB) *Store {
	return &Store{db: db}
}

// GetBool returns the bool value for key, or defaultVal if unset.
func (s *Store) GetBool(key string, defaultVal bool) (bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM config WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultVal, nil
	}
	if err != nil {
		return defaultVal, err
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultVal, nil
	}
	return parsed, nil
}

// SetBool stores a bool config value.
func (s *Store) SetBool(key string, value bool) error {
	_, err := s.db.Exec(
		`INSERT INTO config (key, value) VALUES (?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		key, strconv.FormatBool(value),
	)
	return err
}
