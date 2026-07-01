// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.7 - Phase 6: SQLite-backed DTN bundle queue with expiry and
//           retry tracking.

package dtn

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

// Store persists bundles in the shared dtq_queue table.
type Store struct {
	db *storage.DB
}

// NewStore wraps a migrated storage.DB.
func NewStore(db *storage.DB) *Store {
	return &Store{db: db}
}

// Enqueue stores a bundle for store-and-forward delivery to dst.
func (s *Store) Enqueue(src, dst identity.NodeID, payload []byte, ttl time.Duration) (Bundle, error) {
	var bundleID [16]byte
	if _, err := rand.Read(bundleID[:]); err != nil {
		return Bundle{}, fmt.Errorf("dtn: bundle id: %w", err)
	}
	expires := time.Now().Add(ttl).UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO dtq_queue (bundle_id, src_node_id, dst_node_id, payload, expires_at, retry_count)
		 VALUES (?, ?, ?, ?, ?, 0)`,
		bundleID[:], src[:], dst[:], payload, expires,
	)
	if err != nil {
		return Bundle{}, fmt.Errorf("dtn: enqueue: %w", err)
	}
	return Bundle{
		BundleID:      bundleID,
		SrcNodeID:     src,
		DstNodeID:     dst,
		Payload:       append([]byte(nil), payload...),
		ExpiresAtUnix: expires,
		RetryCount:    0,
	}, nil
}

// List returns every non-expired bundle in the queue.
func (s *Store) List() ([]Bundle, error) {
	now := time.Now().UnixMilli()
	rows, err := s.db.Query(
		`SELECT bundle_id, src_node_id, dst_node_id, payload, expires_at, retry_count
		 FROM dtq_queue WHERE expires_at > ? ORDER BY expires_at`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bundles []Bundle
	for rows.Next() {
		b, err := scanBundle(rows)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, b)
	}
	return bundles, rows.Err()
}

// Delete removes a bundle from the queue.
func (s *Store) Delete(bundleID [16]byte) error {
	_, err := s.db.Exec(`DELETE FROM dtq_queue WHERE bundle_id = ?`, bundleID[:])
	return err
}

// ExpireBefore deletes expired bundles and returns how many were removed.
func (s *Store) ExpireBefore(before time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM dtq_queue WHERE expires_at <= ?`, before.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Depth returns the number of non-expired bundles waiting in the queue.
func (s *Store) Depth() (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM dtq_queue WHERE expires_at > ?`,
		time.Now().UnixMilli(),
	).Scan(&count)
	return count, err
}

// IncrementRetry bumps retry_count for a bundle.
func (s *Store) IncrementRetry(bundleID [16]byte) error {
	_, err := s.db.Exec(
		`UPDATE dtq_queue SET retry_count = retry_count + 1 WHERE bundle_id = ?`,
		bundleID[:],
	)
	return err
}

func scanBundle(rows *sql.Rows) (Bundle, error) {
	var idBytes, srcBytes, dstBytes, payload []byte
	var expires int64
	var retries int
	if err := rows.Scan(&idBytes, &srcBytes, &dstBytes, &payload, &expires, &retries); err != nil {
		return Bundle{}, err
	}
	if len(idBytes) != 16 {
		return Bundle{}, errors.New("dtn: invalid bundle_id length")
	}
	var b Bundle
	copy(b.BundleID[:], idBytes)
	copy(b.SrcNodeID[:], srcBytes)
	copy(b.DstNodeID[:], dstBytes)
	b.Payload = payload
	b.ExpiresAtUnix = expires
	b.RetryCount = uint32(retries)
	return b, nil
}
