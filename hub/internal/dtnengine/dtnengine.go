// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.7 - Phase 6: DTN bundle queue processing — expiry sweep,
//           delivery when a live route appears, depth notifications.

// Package dtnengine runs the hub's store-and-forward queue: bundles
// wait in dtq_queue until a route to their destination exists and the
// destination is online, then they are dequeued (forwarding is a later
// phase). See "Store-and-Forward (DTN Queue)" in /plan.md.
package dtnengine

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/dtn"
	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

// DepthNotifier is called when the non-expired queue depth changes.
type DepthNotifier interface {
	DtnQueueDepthChanged(depth int)
}

// Config configures an Engine.
type Config struct {
	Store    *dtn.Store
	Registry *registry.Registry
	Handler  DepthNotifier
	// TTL is the default lifetime for newly enqueued bundles.
	TTL time.Duration
	// Interval is how often to sweep expiry and attempt delivery.
	Interval time.Duration
}

// Engine processes the DTN bundle queue.
type Engine struct {
	cfg Config

	mu          sync.Mutex
	lastDepth   int
	wg          sync.WaitGroup
	cancel      context.CancelFunc
}

// New creates an Engine. Call Start to begin periodic processing.
func New(cfg Config) *Engine {
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	return &Engine{cfg: cfg, lastDepth: -1}
}

// Start launches the expiry and delivery loop.
func (e *Engine) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.wg.Add(1)
	go e.loop(ctx)
	e.publishDepth()
}

// Close stops the processing loop.
func (e *Engine) Close() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}

// Enqueue stores a bundle for later delivery to dst.
func (e *Engine) Enqueue(src, dst identity.NodeID, payload []byte) error {
	_, err := e.cfg.Store.Enqueue(src, dst, payload, e.cfg.TTL)
	if err != nil {
		return err
	}
	e.publishDepth()
	return nil
}

// OnRouteChanged is called when routing updates; triggers an immediate
// delivery attempt for bundles whose destination may now be reachable.
func (e *Engine) OnRouteChanged(route registry.Route) {
	_ = route
	e.processOnce()
}

func (e *Engine) loop(ctx context.Context) {
	defer e.wg.Done()
	ticker := time.NewTicker(e.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.processOnce()
		}
	}
}

func (e *Engine) processOnce() {
	if _, err := e.cfg.Store.ExpireBefore(time.Now()); err != nil {
		log.Printf("dtnengine: expire: %v", err)
	}
	e.deliverPending()
	e.publishDepth()
}

func (e *Engine) deliverPending() {
	bundles, err := e.cfg.Store.List()
	if err != nil {
		log.Printf("dtnengine: list: %v", err)
		return
	}
	online := e.onlineDestinations()
	for _, b := range bundles {
		var dst identity.NodeID
		copy(dst[:], b.DstNodeID[:])
		if !online[dst] {
			_ = e.cfg.Store.IncrementRetry(b.BundleID)
			continue
		}
		if _, hasRoute, err := e.cfg.Registry.GetRoute(dst); err != nil || !hasRoute {
			_ = e.cfg.Store.IncrementRetry(b.BundleID)
			continue
		}
		if err := e.cfg.Store.Delete(b.BundleID); err != nil {
			log.Printf("dtnengine: delete bundle: %v", err)
		}
	}
}

func (e *Engine) onlineDestinations() map[identity.NodeID]bool {
	nodes, err := e.cfg.Registry.Nodes()
	if err != nil {
		log.Printf("dtnengine: nodes: %v", err)
		return nil
	}
	online := make(map[identity.NodeID]bool, len(nodes))
	for _, n := range nodes {
		if n.Status == registry.NodeStatusOnline {
			online[n.NodeID] = true
		}
	}
	return online
}

func (e *Engine) publishDepth() {
	depth, err := e.cfg.Store.Depth()
	if err != nil {
		log.Printf("dtnengine: depth: %v", err)
		return
	}
	e.mu.Lock()
	changed := depth != e.lastDepth
	e.lastDepth = depth
	handler := e.cfg.Handler
	e.mu.Unlock()
	if changed && handler != nil {
		handler.DtnQueueDepthChanged(depth)
	}
}
