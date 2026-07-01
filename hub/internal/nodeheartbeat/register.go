// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.18 - shared node presence registration for HTTP + LAN beacons.

package nodeheartbeat

import (
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

// RegisterPresence upserts a node in the registry and optionally updates GPS.
func RegisterPresence(
	reg *registry.Registry,
	notifier Notifier,
	nodeID identity.NodeID,
	lat, lon *float64,
	seenAt time.Time,
) (statusChanged bool, err error) {
	changed, err := reg.UpsertSeen(nodeID, seenAt)
	if err != nil {
		return false, err
	}
	if lat != nil && lon != nil {
		_ = reg.UpdateLocation(nodeID, *lat, *lon, seenAt)
	}
	if changed && notifier != nil {
		notifier.NodeStatusChanged(nodeID, registry.NodeStatusOnline)
	}
	return changed, nil
}
