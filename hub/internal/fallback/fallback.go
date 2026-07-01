// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.11 - Phase 9: polls internet_fallback_enabled and emits events.

package fallback

import (
	"context"
	"sync"
	"time"

	"github.com/JohnDovey/QuakeMesh/hub/internal/configstore"
)

// Notifier is called when the internet-fallback toggle changes.
type Notifier interface {
	InternetFallbackChanged(enabled bool)
}

// Engine watches the config table for internet fallback permission changes.
type Engine struct {
	store    *configstore.Store
	notifier Notifier
	interval time.Duration

	mu      sync.Mutex
	last    bool
	hasLast bool

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// New creates an Engine.
func New(store *configstore.Store, notifier Notifier) *Engine {
	return &Engine{store: store, notifier: notifier, interval: 5 * time.Second}
}

// Start begins polling.
func (e *Engine) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.wg.Add(1)
	go e.loop(ctx)
}

// Close stops polling.
func (e *Engine) Close() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}

func (e *Engine) loop(ctx context.Context) {
	defer e.wg.Done()
	e.poll()
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.poll()
		}
	}
}

func (e *Engine) poll() {
	enabled, err := e.store.GetBool(configstore.KeyInternetFallbackEnabled, false)
	if err != nil {
		return
	}
	e.mu.Lock()
	changed := !e.hasLast || enabled != e.last
	if changed {
		e.last = enabled
		e.hasLast = true
		notifier := e.notifier
		e.mu.Unlock()
		if notifier != nil {
			notifier.InternetFallbackChanged(enabled)
		}
		return
	}
	e.mu.Unlock()
}

// Enabled returns the current internet-fallback permission.
func (e *Engine) Enabled() bool {
	enabled, _ := e.store.GetBool(configstore.KeyInternetFallbackEnabled, false)
	return enabled
}
