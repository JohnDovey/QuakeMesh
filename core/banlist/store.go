// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.13 - Phase 11: ban_list and ban_verdicts access with gossip merge.

// Package banlist stores app ban proposals and per-hub verdicts.
package banlist

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

// Proposal is a row from ban_list.
type Proposal struct {
	BanID         [16]byte
	AppID         string
	VersionRange  string
	Reason        string
	ProposedBy    identity.NodeID
	ProposedAt    time.Time
	Signature     []byte
}

// Verdict is a row from ban_verdicts.
type Verdict struct {
	BanID     [16]byte
	HubID     identity.NodeID
	Agree     bool
	DecidedAt time.Time
}

// Tally holds agree/disagree counts for a proposal.
type Tally struct {
	Agree    int
	Disagree int
}

// Store reads and writes ban_list / ban_verdicts.
type Store struct {
	db *storage.DB
}

// NewStore wraps a migrated storage.DB.
func NewStore(db *storage.DB) *Store {
	return &Store{db: db}
}

// Propose inserts a new ban proposal.
func (s *Store) Propose(p Proposal) error {
	if p.AppID == "" {
		return fmt.Errorf("banlist: app_id required")
	}
	if len(p.BanID) == 0 {
		if _, err := rand.Read(p.BanID[:]); err != nil {
			return err
		}
	}
	if p.VersionRange == "" {
		p.VersionRange = "*"
	}
	if p.Signature == nil {
		p.Signature = []byte{}
	}
	_, err := s.db.Exec(
		`INSERT INTO ban_list (ban_id, app_id, version_range, reason, proposed_by_hub_id, proposed_at, signature)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.BanID[:], p.AppID, p.VersionRange, p.Reason, p.ProposedBy[:],
		p.ProposedAt.UnixMilli(), p.Signature,
	)
	if err != nil {
		return fmt.Errorf("banlist: propose: %w", err)
	}
	return nil
}

// UpdateSignature sets the signature for a proposal (hub signing pass).
func (s *Store) UpdateSignature(banID [16]byte, signature []byte) error {
	_, err := s.db.Exec(`UPDATE ban_list SET signature = ? WHERE ban_id = ?`, signature, banID[:])
	return err
}

// ListProposals returns every ban proposal.
func (s *Store) ListProposals() ([]Proposal, error) {
	rows, err := s.db.Query(
		`SELECT ban_id, app_id, version_range, reason, proposed_by_hub_id, proposed_at, signature
		 FROM ban_list ORDER BY proposed_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProposals(rows)
}

// ListUnsignedByProposer returns proposals from hubID with an empty signature.
func (s *Store) ListUnsignedByProposer(hubID identity.NodeID) ([]Proposal, error) {
	rows, err := s.db.Query(
		`SELECT ban_id, app_id, version_range, reason, proposed_by_hub_id, proposed_at, signature
		 FROM ban_list WHERE proposed_by_hub_id = ? AND (signature IS NULL OR length(signature) = 0)`,
		hubID[:],
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProposals(rows)
}

// SetVerdict records this hub's agree/disagree decision.
func (s *Store) SetVerdict(v Verdict) error {
	agree := 0
	if v.Agree {
		agree = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO ban_verdicts (ban_id, hub_id, agree, decided_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (ban_id, hub_id) DO UPDATE SET agree = excluded.agree, decided_at = excluded.decided_at`,
		v.BanID[:], v.HubID[:], agree, v.DecidedAt.UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("banlist: verdict: %w", err)
	}
	return nil
}

// VerdictForHub returns one hub's verdict on a ban, if any.
func (s *Store) VerdictForHub(banID [16]byte, hubID identity.NodeID) (Verdict, bool, error) {
	var agree int
	var decidedMs int64
	err := s.db.QueryRow(
		`SELECT agree, decided_at FROM ban_verdicts WHERE ban_id = ? AND hub_id = ?`,
		banID[:], hubID[:],
	).Scan(&agree, &decidedMs)
	if errors.Is(err, sql.ErrNoRows) {
		return Verdict{}, false, nil
	}
	if err != nil {
		return Verdict{}, false, err
	}
	return Verdict{
		BanID: banID, HubID: hubID, Agree: agree != 0,
		DecidedAt: time.UnixMilli(decidedMs),
	}, true, nil
}

// TallyVerdicts counts agree/disagree verdicts for a ban.
func (s *Store) TallyVerdicts(banID [16]byte) (Tally, error) {
	var t Tally
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM ban_verdicts WHERE ban_id = ? AND agree = 1`, banID[:],
	).Scan(&t.Agree); err != nil {
		return t, err
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM ban_verdicts WHERE ban_id = ? AND agree = 0`, banID[:],
	).Scan(&t.Disagree); err != nil {
		return t, err
	}
	return t, nil
}

// ListAllVerdicts returns every ban_verdicts row.
func (s *Store) ListAllVerdicts() ([]Verdict, error) {
	rows, err := s.db.Query(`SELECT ban_id, hub_id, agree, decided_at FROM ban_verdicts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var verdicts []Verdict
	for rows.Next() {
		var banBytes, hubBytes []byte
		var agree int
		var decidedMs int64
		if err := rows.Scan(&banBytes, &hubBytes, &agree, &decidedMs); err != nil {
			return nil, err
		}
		if len(banBytes) != 16 {
			continue
		}
		var banID [16]byte
		copy(banID[:], banBytes)
		var hubID identity.NodeID
		copy(hubID[:], hubBytes)
		verdicts = append(verdicts, Verdict{
			BanID: banID, HubID: hubID, Agree: agree != 0,
			DecidedAt: time.UnixMilli(decidedMs),
		})
	}
	return verdicts, rows.Err()
}

// ListVerdicts returns every verdict for a ban.
func (s *Store) ListVerdicts(banID [16]byte) ([]Verdict, error) {
	rows, err := s.db.Query(
		`SELECT hub_id, agree, decided_at FROM ban_verdicts WHERE ban_id = ?`,
		banID[:],
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var verdicts []Verdict
	for rows.Next() {
		var hubBytes []byte
		var agree int
		var decidedMs int64
		if err := rows.Scan(&hubBytes, &agree, &decidedMs); err != nil {
			return nil, err
		}
		var hubID identity.NodeID
		copy(hubID[:], hubBytes)
		verdicts = append(verdicts, Verdict{
			BanID: banID, HubID: hubID, Agree: agree != 0,
			DecidedAt: time.UnixMilli(decidedMs),
		})
	}
	return verdicts, rows.Err()
}

// IsLocallyEnforced reports whether localHub has agreed to enforce a ban
// matching appID@version.
func (s *Store) IsLocallyEnforced(localHub identity.NodeID, appID, version string) (bool, error) {
	proposals, err := s.ListProposals()
	if err != nil {
		return false, err
	}
	for _, p := range proposals {
		if p.AppID != appID || !VersionInRange(version, p.VersionRange) {
			continue
		}
		v, ok, err := s.VerdictForHub(p.BanID, localHub)
		if err != nil {
			return false, err
		}
		if ok && v.Agree {
			return true, nil
		}
	}
	return false, nil
}

// MergeGossipProposal inserts or updates a gossiped proposal if newer.
func (s *Store) MergeGossipProposal(p Proposal) (bool, error) {
	proposedMs := p.ProposedAt.UnixMilli()
	var localMs int64
	err := s.db.QueryRow(`SELECT proposed_at FROM ban_list WHERE ban_id = ?`, p.BanID[:]).Scan(&localMs)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return true, s.Propose(p)
	case err != nil:
		return false, err
	default:
		if proposedMs < localMs {
			return false, nil
		}
		_, err = s.db.Exec(
			`UPDATE ban_list SET app_id = ?, version_range = ?, reason = ?, proposed_by_hub_id = ?,
			 proposed_at = ?, signature = ? WHERE ban_id = ?`,
			p.AppID, p.VersionRange, p.Reason, p.ProposedBy[:], proposedMs, p.Signature, p.BanID[:],
		)
		return err == nil, err
	}
}

// MergeGossipVerdict applies a gossiped verdict if newer.
func (s *Store) MergeGossipVerdict(v Verdict) (bool, error) {
	decidedMs := v.DecidedAt.UnixMilli()
	var localMs int64
	err := s.db.QueryRow(
		`SELECT decided_at FROM ban_verdicts WHERE ban_id = ? AND hub_id = ?`,
		v.BanID[:], v.HubID[:],
	).Scan(&localMs)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return true, s.SetVerdict(v)
	case err != nil:
		return false, err
	default:
		if decidedMs <= localMs {
			return false, nil
		}
		return true, s.SetVerdict(v)
	}
}

// SignBytes returns the canonical message bytes for proposal signing.
func SignBytes(p Proposal) []byte {
	return []byte(fmt.Sprintf("%x|%s|%s|%s|%d",
		p.BanID[:], p.AppID, p.VersionRange, p.Reason, p.ProposedAt.UnixMilli()))
}

// VersionInRange returns true when version matches range (*, exact, or prefix*).
func VersionInRange(version, versionRange string) bool {
	versionRange = strings.TrimSpace(versionRange)
	if versionRange == "" || versionRange == "*" {
		return true
	}
	if strings.HasSuffix(versionRange, ".*") || strings.HasSuffix(versionRange, ".x") {
		prefix := strings.TrimSuffix(strings.TrimSuffix(versionRange, ".*"), ".x")
		return strings.HasPrefix(version, prefix)
	}
	if strings.HasSuffix(versionRange, "*") {
		return strings.HasPrefix(version, strings.TrimSuffix(versionRange, "*"))
	}
	return version == versionRange
}

func scanProposals(rows *sql.Rows) ([]Proposal, error) {
	var proposals []Proposal
	for rows.Next() {
		var idBytes, proposerBytes, sig []byte
		var appID, versionRange, reason string
		var proposedMs int64
		if err := rows.Scan(&idBytes, &appID, &versionRange, &reason, &proposerBytes, &proposedMs, &sig); err != nil {
			return nil, err
		}
		if len(idBytes) != 16 {
			continue
		}
		var p Proposal
		copy(p.BanID[:], idBytes)
		p.AppID = appID
		p.VersionRange = versionRange
		p.Reason = reason
		copy(p.ProposedBy[:], proposerBytes)
		p.ProposedAt = time.UnixMilli(proposedMs)
		p.Signature = sig
		proposals = append(proposals, p)
	}
	return proposals, rows.Err()
}

// BanIDEqual compares two ban IDs.
func BanIDEqual(a, b [16]byte) bool {
	return bytes.Equal(a[:], b[:])
}
