// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Initial Phase 2 orchestration: identity + registry + OGM
//           engine + management API wired into one runnable Hub.
//   0.0.7 - Phase 6: DTN store-and-forward engine wired into hub lifecycle.
//   0.0.11 - Phase 9: gossip sync, metrics, internet-fallback watcher.
//   0.0.12 - Phase 10: local app SDK daemon API on Unix socket.

// Package hubapp wires identity, the SQLite registry, the OGM engine,
// the DTN engine, gossip sync, and the loopback management API into a
// single runnable QuakeMeshHub instance. See "QuakeMeshHub" and Phase 2
// in /plan.md.
package hubapp

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/apppresence"
	"github.com/JohnDovey/QuakeMesh/core/dtn"
	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/metrics"
	"github.com/JohnDovey/QuakeMesh/core/storage"
	"github.com/JohnDovey/QuakeMesh/core/trust"
	"github.com/JohnDovey/QuakeMesh/hub/internal/configstore"
	"github.com/JohnDovey/QuakeMesh/hub/internal/daemonapi"
	"github.com/JohnDovey/QuakeMesh/hub/internal/dtnengine"
	"github.com/JohnDovey/QuakeMesh/hub/internal/fallback"
	"github.com/JohnDovey/QuakeMesh/hub/internal/managementapi"
	"github.com/JohnDovey/QuakeMesh/hub/internal/ogmengine"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
	"github.com/JohnDovey/QuakeMesh/hub/internal/syncengine"
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
	// SyncInterval is how often to gossip HubSyncMessage to peers.
	SyncInterval time.Duration
	// AppSocket is the mesh-sdk daemon listen address (unix: or tcp:).
	// Empty disables the local app API.
	AppSocket string
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
		SyncInterval: 30 * time.Second,
		AppSocket:    "unix:/tmp/quakemeshhub.sock",
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
	Sync     *syncengine.Engine
	Fallback *fallback.Engine
	API      *managementapi.Server
	Daemon   *daemonapi.Server
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
	cfgStore := configstore.New(db)
	metricsStore := metrics.NewStore(db)
	appStore := apppresence.NewStore(db)
	dtnStore := dtn.NewStore(db)
	trustStore := trust.NewStore(db)
	dtnEng := dtnengine.New(dtnengine.Config{
		Store:    dtnStore,
		Registry: reg,
		Handler:  api,
		SelfID:   id.NodeID,
		TTL:      cfg.DTNTTL,
		Interval: cfg.DTNInterval,
	})
	var daemon *daemonapi.Server
	if cfg.AppSocket != "" {
		daemon = daemonapi.New(daemonapi.Config{
			SelfID:     id.NodeID,
			ListenAddr: cfg.AppSocket,
			Apps:       appStore,
			Sender:     dtnEng,
			Notifier:   api,
		})
		dtnEng.SetLocalDeliverer(daemon)
	}
	fallbackEng := fallback.New(cfgStore, api)
	events := &eventBridge{api: api, dtn: dtnEng}
	syncBind, err := syncengine.SyncBindAddr(cfg.OGMBindAddr)
	if err != nil {
		return nil, fmt.Errorf("hubapp: sync bind addr: %w", err)
	}
	syncEng := syncengine.New(syncengine.Config{
		SelfID:   id.NodeID,
		BindAddr: syncBind,
		Peers:    syncengine.PeerSyncAddrs(cfg.Peers),
		Interval: cfg.SyncInterval,
		Registry: reg,
		Apps:     appStore,
	})
	ogm := ogmengine.New(ogmengine.Config{
		SelfID:     id.NodeID,
		BindAddr:   cfg.OGMBindAddr,
		Peers:      cfg.Peers,
		Interval:   cfg.OGMInterval,
		StaleAfter: cfg.StaleAfter,
		TTL:        cfg.OGMTTL,
		Registry:   reg,
		Trust:      trustStore,
		Metrics:    metricsStore,
		Handler:    events,
	})

	return &Hub{
		cfg: cfg, Identity: id, DB: db, Registry: reg,
		OGM: ogm, DTN: dtnEng, Sync: syncEng, Fallback: fallbackEng, API: api, Daemon: daemon,
	}, nil
}

// Start begins the management API's HTTP server, the DTN engine, and the
// OGM engine.
func (h *Hub) Start() error {
	if err := h.API.Start(); err != nil {
		return fmt.Errorf("hubapp: start management API: %w", err)
	}
	if h.Daemon != nil {
		if err := h.Daemon.Start(); err != nil {
			h.API.Close()
			return fmt.Errorf("hubapp: start app daemon: %w", err)
		}
	}
	h.DTN.Start()
	h.Fallback.Start()
	if err := h.Sync.Start(); err != nil {
		h.Fallback.Close()
		h.DTN.Close()
		if h.Daemon != nil {
			h.Daemon.Close()
		}
		h.API.Close()
		return fmt.Errorf("hubapp: start sync engine: %w", err)
	}
	if err := h.OGM.Start(); err != nil {
		h.Sync.Close()
		h.Fallback.Close()
		h.DTN.Close()
		if h.Daemon != nil {
			h.Daemon.Close()
		}
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
	h.Fallback.Close()
	h.DTN.Close()
	var daemonErr error
	if h.Daemon != nil {
		daemonErr = h.Daemon.Close()
	}
	return errors.Join(h.Sync.Close(), h.OGM.Close(), daemonErr, h.API.Close(), h.DB.Close())
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
