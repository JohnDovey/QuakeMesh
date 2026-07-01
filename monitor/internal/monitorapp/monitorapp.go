// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.4 - Phase 3 orchestration: shared Hub SQLite, auth seeding,
//           hub event client, and HTTP dashboard server.

package monitorapp

import (
	"context"
	"embed"
	"fmt"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/storage"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/auth"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/datastore"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/hubclient"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/server"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/sosfeed"
)

// Config configures a runnable Monitor instance.
type Config struct {
	BindAddr  string
	HubDBPath string
	HubWSURL  string
}

// Monitor is a running QuakeMeshMonitor instance.
type Monitor struct {
	cfg    Config
	DB     *storage.DB
	Auth   *auth.Store
	Data   *datastore.Store
	Hub    *hubclient.Client
	Server *server.Server

	cancel context.CancelFunc
}

// New opens the Hub's SQLite database, seeds the default admin account,
// and wires the dashboard server.
func New(cfg Config, staticFS embed.FS) (*Monitor, error) {
	db, err := storage.Open(cfg.HubDBPath)
	if err != nil {
		return nil, fmt.Errorf("monitorapp: open hub db: %w", err)
	}

	authStore := auth.New(db)
	if err := authStore.EnsureDefaultAdmin(); err != nil {
		db.Close()
		return nil, fmt.Errorf("monitorapp: seed admin: %w", err)
	}

	data := datastore.New(db)
	sos := sosfeed.New()
	hub := hubclient.New(cfg.HubWSURL)
	srv := server.New(server.Config{
		BindAddr: cfg.BindAddr,
		StaticFS: staticFS,
		Auth:     authStore,
		Data:     data,
		Hub:      hub,
		SOS:      sos,
	})

	return &Monitor{
		cfg:    cfg,
		DB:     db,
		Auth:   authStore,
		Data:   data,
		Hub:    hub,
		Server: srv,
	}, nil
}

// Start begins the HTTP server and Hub event consumer.
func (m *Monitor) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.Hub.Run(ctx)
	return m.Server.Start()
}

// Addr returns the HTTP bind address after Start.
func (m *Monitor) Addr() string {
	if m.Server.Addr() == nil {
		return m.cfg.BindAddr
	}
	return m.Server.Addr().String()
}

// Close stops background workers and releases resources.
func (m *Monitor) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Server.Close(ctx); err != nil {
		return err
	}
	return m.DB.Close()
}
