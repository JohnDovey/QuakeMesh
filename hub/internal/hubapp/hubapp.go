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
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/apppresence"
	"github.com/JohnDovey/QuakeMesh/core/banlist"
	"github.com/JohnDovey/QuakeMesh/core/dtn"
	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/lansegments"
	"github.com/JohnDovey/QuakeMesh/core/metrics"
	"github.com/JohnDovey/QuakeMesh/core/storage"
	"github.com/JohnDovey/QuakeMesh/core/trust"
	"github.com/JohnDovey/QuakeMesh/hub/internal/configstore"
	"github.com/JohnDovey/QuakeMesh/hub/internal/daemonapi"
	"github.com/JohnDovey/QuakeMesh/hub/internal/dtnengine"
	"github.com/JohnDovey/QuakeMesh/hub/internal/fallback"
	"github.com/JohnDovey/QuakeMesh/hub/internal/landiscovery"
	"github.com/JohnDovey/QuakeMesh/hub/internal/managementapi"
	"github.com/JohnDovey/QuakeMesh/hub/internal/nodeheartbeat"
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
	// LAN multicast discovery supplements this list for mesh nodes.
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
	// HeartbeatAddr is the LAN HTTP bind address for mesh node presence
	// reports (e.g. "0.0.0.0:18085"). Empty disables.
	HeartbeatAddr string
	// DiscoveryBind is the UDP bind for LAN multicast beacons (e.g.
	// "0.0.0.0:47223"). Empty disables.
	DiscoveryBind string
	// AppVersion is advertised on GET /sniff (MeshSniff / LAN identity).
	AppVersion string
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
		HeartbeatAddr: "0.0.0.0:18085",
		DiscoveryBind: "0.0.0.0:47223",
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
	Heartbeat *nodeheartbeat.Server
	Discovery *landiscovery.Engine

	cancel context.CancelFunc
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
	segmentStore := lansegments.NewStore(db)
	api := managementapi.New(cfg.ManagementAddr)
	cfgStore := configstore.New(db)
	metricsStore := metrics.NewStore(db)
	appStore := apppresence.NewStore(db)
	banStore := banlist.NewStore(db)
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
			Bans:       banStore,
			LocalHub:   id.NodeID,
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
		Bans:     banStore,
		LocalHub: id.NodeID,
		SignPending: func() {
			_ = signPendingProposals(id, banStore)
		},
		BanNotify: api,
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

	heartbeatPort, _ := portFromAddr(cfg.HeartbeatAddr)
	ogmPort, _ := portFromAddr(cfg.OGMBindAddr)
	mgmtPort, _ := portFromAddr(cfg.ManagementAddr)
	discoveryPort, _ := portFromAddr(cfg.DiscoveryBind)

	api.SetSniff(managementapi.SniffConfig{
		MeshID:         id.NodeID.String(),
		AppVersion:     cfg.AppVersion,
		HeartbeatPort:  heartbeatPort,
		ManagementPort: mgmtPort,
		OGMPort:        ogmPort,
		DiscoveryPort:  discoveryPort,
	})

	var heartbeat *nodeheartbeat.Server
	if cfg.HeartbeatAddr != "" {
		heartbeat = nodeheartbeat.New(nodeheartbeat.Config{
			ListenAddr:     cfg.HeartbeatAddr,
			Registry:       reg,
			Notifier:       events,
			SOSNotifier:    api,
			Segments:       segmentStore,
			Trust:          trustStore,
			LocalHub:       id.NodeID,
			AppVersion:     cfg.AppVersion,
			HeartbeatPort:  heartbeatPort,
			ManagementPort: mgmtPort,
			OGMPort:        ogmPort,
			DiscoveryPort:  discoveryPort,
		})
	}

	var discovery *landiscovery.Engine
	if cfg.DiscoveryBind != "" && heartbeatPort > 0 && ogmPort > 0 {
		discovery = landiscovery.New(landiscovery.Config{
			BindAddr:      cfg.DiscoveryBind,
			HubNodeID:     id.NodeID,
			HeartbeatPort: heartbeatPort,
			OGMPort:       ogmPort,
			Registry:      reg,
			Notifier:      events,
			HubNotifier:   events,
			Segments:      segmentStore,
		})
	}

	return &Hub{
		cfg: cfg, Identity: id, DB: db, Registry: reg,
		OGM: ogm, DTN: dtnEng, Sync: syncEng, Fallback: fallbackEng, API: api, Daemon: daemon,
		Heartbeat: heartbeat, Discovery: discovery,
	}, nil
}

// Start begins the management API's HTTP server, the DTN engine, and the
// OGM engine.
func (h *Hub) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	if err := h.API.Start(); err != nil {
		return fmt.Errorf("hubapp: start management API: %w", err)
	}
	if h.Daemon != nil {
		if err := h.Daemon.Start(); err != nil {
			h.API.Close()
			return fmt.Errorf("hubapp: start app daemon: %w", err)
		}
	}
	if h.Heartbeat != nil {
		if err := h.Heartbeat.Start(); err != nil {
			if h.Daemon != nil {
				h.Daemon.Close()
			}
			h.API.Close()
			return fmt.Errorf("hubapp: start node heartbeat: %w", err)
		}
	}
	if h.Discovery != nil {
		if err := h.Discovery.Start(); err != nil {
			if h.Heartbeat != nil {
				h.Heartbeat.Close()
			}
			if h.Daemon != nil {
				h.Daemon.Close()
			}
			h.API.Close()
			return fmt.Errorf("hubapp: start LAN discovery: %w", err)
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
	go h.selfHubLivenessLoop(ctx)
	if err := signPendingProposals(h.Identity, banlist.NewStore(h.DB)); err != nil {
		return fmt.Errorf("hubapp: sign ban proposals: %w", err)
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
	_ = configstore.New(h.DB).Set(configstore.KeyLocalHubID, h.Identity.NodeID.String())
	return nil
}

// selfHubLivenessLoop keeps this hub's hub_registry row fresh so Monitor
// does not mark it stale after the OGM stale-after window.
func (h *Hub) selfHubLivenessLoop(ctx context.Context) {
	interval := h.cfg.OGMInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := h.registerSelfHub(); err != nil {
				log.Printf("hubapp: self hub liveness: %v", err)
			}
		}
	}
}

func portFromAddr(addr string) (int, error) {
	if addr == "" {
		return 0, nil
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return 0, err
	}
	return port, nil
}

func signPendingProposals(id *identity.Identity, bans *banlist.Store) error {
	pending, err := bans.ListUnsignedByProposer(id.NodeID)
	if err != nil {
		return err
	}
	for _, p := range pending {
		sig := id.Sign(banlist.SignBytes(p))
		if err := bans.UpdateSignature(p.BanID, sig); err != nil {
			return err
		}
	}
	return nil
}

// Close stops the OGM engine, DTN engine, and management API and closes
// the registry database.
func (h *Hub) Close() error {
	if h.cancel != nil {
		h.cancel()
	}
	h.Fallback.Close()
	h.DTN.Close()
	var daemonErr error
	if h.Daemon != nil {
		daemonErr = h.Daemon.Close()
	}
	var heartbeatErr error
	if h.Heartbeat != nil {
		heartbeatErr = h.Heartbeat.Close()
	}
	var discoveryErr error
	if h.Discovery != nil {
		discoveryErr = h.Discovery.Close()
	}
	return errors.Join(h.Sync.Close(), h.OGM.Close(), daemonErr, heartbeatErr, discoveryErr, h.API.Close(), h.DB.Close())
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
