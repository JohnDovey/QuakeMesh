// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Initial Phase 2 orchestration: identity + registry + OGM
//           engine + management API wired into one runnable Hub.
//   0.0.7 - Phase 6: DTN store-and-forward engine wired into hub lifecycle.

// Package hubapp wires identity, the SQLite registry, the OGM engine,
// the DTN engine, and the loopback management API into a single runnable
// QuakeMeshHub instance. See "QuakeMeshHub" and Phase 2 in /plan.md.
package hubapp

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/dtn"
	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
	"github.com/JohnDovey/QuakeMesh/core/trust"
	"github.com/JohnDovey/QuakeMesh/hub/internal/dtnengine"
	"github.com/JohnDovey/QuakeMesh/hub/internal/managementapi"
	"github.com/JohnDovey/QuakeMesh/hub/internal/ogmengine"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

// Config configures a runnable Hub instance.
type Config struct {
	// IdentityPath is where this hub's Ed25519 seed is persisted,
	// generated on first run.
	IdentityPath string
	// DBPath is the SQLite registry file (quakemeshhub.db).
	DBPath string
	// OGMBindAddr is the local UDP address for OGM exchange, e.g.
	// "0.0.0.0:47222".
	OGMBindAddr string
	// Peers is the static list of other hubs' OGM UDP addresses.
	// Automatic LAN discovery is a later phase.
	Peers []string
	// OGMInterval is how often this hub broadcasts an OGM.
	OGMInterval time.Duration
	// StaleAfter is how long without a received OGM before a peer is
	// marked stale.
	StaleAfter time.Duration
	// OGMTTL is the OGM's initial hop budget.
	OGMTTL uint32
	// ManagementAddr is the loopback management API's bind address,
	// e.g. "127.0.0.1:8083".
	ManagementAddr string
	// DTNTTL is the default lifetime for queued bundles.
	DTNTTL time.Duration
	// DTNInterval is how often the DTN engine sweeps expiry and attempts
	// delivery.
	DTNInterval time.Duration
}

// DefaultConfig returns sensible defaults for the tuning parameters.
// Paths and addresses are deployment-specific and always left to the
// caller.
func DefaultConfig() Config {
	return Config{
		OGMInterval: 5 * time.Second,
		StaleAfter:  20 * time.Second,
		OGMTTL:      3,
		DTNTTL:      24 * time.Hour,
		DTNInterval: 5 * time.Second,
	}
}

// Hub is a single running QuakeMeshHub instance.
type Hub struct {
	cfg Config

	Identity *identity.Identity
	DB       *storage.DB
	Registry *registry.Registry
	OGM      *ogmengine.Engine
	DTN      *dtnengine.Engine
	API      *managementapi.Server
}

// New loads or creates this hub's identity, opens (and migrates) its
// registry database, and wires the OGM engine and management API
// together. Call Start to begin listening.
func New(cfg Config) (*Hub, error) {
	id, err := identity.LoadOrCreate(cfg.IdentityPath)
	if err != nil {
		return nil, fmt.Errorf("hubapp: load identity: %w", err)
	}

	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("hubapp: open registry: %w", err)
	}

	reg := registry.New(db)
	api := managementapi.New(cfg.ManagementAddr)
	dtnStore := dtn.NewStore(db)
	trustStore := trust.NewStore(db)
	dtnEng := dtnengine.New(dtnengine.Config{
		Store:    dtnStore,
		Registry: reg,
		Handler:  api,
		TTL:      cfg.DTNTTL,
		Interval: cfg.DTNInterval,
	})
	events := &eventBridge{api: api, dtn: dtnEng}
	ogm := ogmengine.New(ogmengine.Config{
		SelfID:     id.NodeID,
		BindAddr:   cfg.OGMBindAddr,
		Peers:      cfg.Peers,
		Interval:   cfg.OGMInterval,
		StaleAfter: cfg.StaleAfter,
		TTL:        cfg.OGMTTL,
		Registry:   reg,
		Trust:      trustStore,
		Handler:    events,
	})

	return &Hub{cfg: cfg, Identity: id, DB: db, Registry: reg, OGM: ogm, DTN: dtnEng, API: api}, nil
}

// Start begins the management API's HTTP server, the DTN engine, and the
// OGM engine.
func (h *Hub) Start() error {
	if err := h.API.Start(); err != nil {
		return fmt.Errorf("hubapp: start management API: %w", err)
	}
	h.DTN.Start()
	if err := h.OGM.Start(); err != nil {
		h.DTN.Close()
		h.API.Close()
		return fmt.Errorf("hubapp: start OGM engine: %w", err)
	}
	if err := h.registerSelfHub(); err != nil {
		return fmt.Errorf("hubapp: register self hub: %w", err)
	}
	return nil
}

func (h *Hub) registerSelfHub() error {
	addr, ok := h.OGM.LocalAddr().(*net.UDPAddr)
	if !ok {
		return fmt.Errorf("unexpected OGM local addr type %T", h.OGM.LocalAddr())
	}
	changed, err := h.Registry.UpsertHubSeen(h.Identity.NodeID, addr.IP.String(), addr.Port, time.Now())
	if err != nil {
		return err
	}
	if changed {
		h.API.HubStatusChanged(h.Identity.NodeID, registry.HubStatusOnline)
	}
	return nil
}

// Close stops the OGM engine, DTN engine, and management API and closes
// the registry database.
func (h *Hub) Close() error {
	h.DTN.Close()
	return errors.Join(h.OGM.Close(), h.API.Close(), h.DB.Close())
}

type eventBridge struct {
	api *managementapi.Server
	dtn *dtnengine.Engine
}

func (b *eventBridge) NodeStatusChanged(nodeID identity.NodeID, status registry.NodeStatus) {
	b.api.NodeStatusChanged(nodeID, status)
}

func (b *eventBridge) HubStatusChanged(hubID identity.NodeID, status registry.HubStatus) {
	b.api.HubStatusChanged(hubID, status)
}

func (b *eventBridge) RouteChanged(route registry.Route) {
	b.api.RouteChanged(route)
	b.dtn.OnRouteChanged(route)
}
